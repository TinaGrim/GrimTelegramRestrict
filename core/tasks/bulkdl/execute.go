// Package bulkdl implements channel-wide media bulk download for SaveAny-Bot
// (https://github.com/krau/SaveAny-Bot) by krau.
//
// The download strategy, media type classification, checkpoint/resume and
// failed-retry semantics are ported from telegram_media_downloader
// (https://github.com/Dineshkarthik/telegram_media_downloader)
// by Dineshkarthik (AGPL-3.0), in accordance with SaveAny-Bot's AGPL-3.0
// license.
//
// Actual file transfer (download, upload, retries, progress rendering) is
// delegated entirely to the batch Telegram-file engine (core/tasks/batchtfile),
// which gives every discovered page the familiar "📦 Processing" card with
// per-file bars and speeds.
package bulkdl

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gotd/td/tg"

	"github.com/krau/SaveAny-Bot/common/utils/tgutil"
	"github.com/krau/SaveAny-Bot/config"
	"github.com/krau/SaveAny-Bot/core/tasks/batchtfile"
	"github.com/krau/SaveAny-Bot/database"
	"github.com/krau/SaveAny-Bot/pkg/taskevent"
	"github.com/krau/SaveAny-Bot/pkg/tfile"
)

const (
	// pageSize is the number of history messages fetched per request,
	// mirroring telegram_media_downloader's pagination_limit.
	pageSize = 100
	// maxPersistedFailedIDs caps the persisted failed-ID list to keep the
	// checkpoint row small.
	maxPersistedFailedIDs = 500
	// maxCollectBlocks bounds how many history pages may be buffered between
	// the checkpoint and the present before giving up.
	maxCollectBlocks = 1000
	// deadConnectionMinFailures is how many files must fail in one block
	// (with zero successes) before the run concludes the link is down.
	deadConnectionMinFailures = 5
)

// Execute implements core.Executable.
func (t *Task) Execute(ctx context.Context) error {
	logger := log.FromContext(ctx).WithPrefix(fmt.Sprintf("bulkdl[%s]", t.ID))
	logger.Infof("Starting bulk download from chat %d (%s)", t.Params.ChatID, t.Params.ChatLabel)
	stats := newStats()
	lastRead := t.loadCheckpoint(ctx, logger)
	t.resume = lastRead
	if t.Progress != nil {
		t.Progress.OnStart(ctx, t)
	}

	if err := t.retryFailedMessages(ctx, logger, stats); err != nil && !errors.Is(err, context.Canceled) {
		logger.Errorf("Failed to retry previously failed messages: %v", err)
	}

	finalLastRead, err := t.iterateHistory(ctx, logger, stats, lastRead)
	t.saveCheckpoint(ctx, logger, finalLastRead)

	// A transport layer below us (gotd recovery) may fail requests with
	// errors that wrap context.Canceled even though the task itself was
	// never canceled. Only treat it as a cancellation when OUR context is
	// actually done; otherwise surface it as a regular failure so the
	// checkpoint lets the next run resume.
	if err != nil && errors.Is(err, context.Canceled) && ctx.Err() == nil {
		err = fmt.Errorf("bulk download interrupted (resumes from checkpoint on next run): %v", err)
	}

	if t.Progress != nil {
		t.Progress.OnDone(ctx, t, err, stats.Snapshot())
	}
	return err
}

func (t *Task) loadCheckpoint(ctx context.Context, logger *log.Logger) int {
	progress, err := database.GetBulkDLProgress(ctx, t.Params.UserID, t.Params.ChatID)
	if err != nil {
		logger.Errorf("Failed to load bulk download progress: %v", err)
		return 0
	}
	if progress == nil {
		return 0
	}
	t.failedMu.Lock()
	for _, id := range database.ParseFailedIDs(progress.FailedIDs) {
		t.failed[id] = struct{}{}
	}
	count := len(t.failed)
	t.failedMu.Unlock()
	logger.Infof("Resuming from message ID %d with %d previously failed message(s)", progress.LastReadMessageID, count)
	return progress.LastReadMessageID
}

// saveCheckpoint persists the resume point along with the current failure
// set so failed files are retried on the next run.
func (t *Task) saveCheckpoint(ctx context.Context, logger *log.Logger, lastRead int) {
	ids := make([]int, 0, len(t.failed))
	t.failedMu.Lock()
	for id := range t.failed {
		ids = append(ids, id)
	}
	t.failedMu.Unlock()
	sort.Ints(ids)
	if len(ids) > maxPersistedFailedIDs {
		ids = ids[len(ids)-maxPersistedFailedIDs:]
	}
	if err := database.SaveBulkDLProgress(ctx, t.Params.UserID, t.Params.ChatID, lastRead, ids); err != nil {
		logger.Errorf("Failed to save bulk download progress: %v", err)
	}
}

// retryFailedMessages re-downloads messages that failed during previous runs,
// mirroring telegram_media_downloader's ids_to_retry behavior.
func (t *Task) retryFailedMessages(ctx context.Context, logger *log.Logger, stats *Stats) error {
	t.failedMu.Lock()
	ids := make([]int, 0, len(t.failed))
	for id := range t.failed {
		ids = append(ids, id)
	}
	t.failedMu.Unlock()
	if len(ids) == 0 {
		return nil
	}
	logger.Infof("Retrying %d previously failed message(s)", len(ids))
	sort.Ints(ids)
	for _, id := range ids {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg, err := tgutil.GetMessageByID(t.UserCtx, t.Params.ChatID, id)
		if err != nil {
			logger.Errorf("Failed to refetch message %d: %v", id, err)
			continue
		}
		stats.MarkScanned()
		if _, _, rerr := t.runBlock(ctx, logger, []*tg.Message{msg}, stats); rerr != nil && !errors.Is(rerr, context.Canceled) {
			logger.Errorf("Retry of message %d failed: %v", id, rerr)
		}
	}
	return nil
}

// iterateHistory processes the channel history in ascending order (oldest to
// newest), starting just after the resume checkpoint — the same direction as
// telegram_media_downloader's reverse=True iteration.
//
// Implementation: history is collected with plain newest-first paging down to
// the checkpoint, buffered, then replayed in reverse. Each buffered page is
// handed to the batch file engine as one unit; the checkpoint only ever
// advances over fully processed contiguous pages. It returns the resume point
// to persist for the next run.
func (t *Task) iterateHistory(ctx context.Context, logger *log.Logger, stats *Stats, lastRead int) (int, error) {
	peer, err := t.UserCtx.ResolveInputPeerById(t.Params.ChatID)
	if err != nil {
		return lastRead, fmt.Errorf("failed to resolve chat %d: %w", t.Params.ChatID, err)
	}

	var blocks [][]*tg.Message
	offsetID := 0
	for {
		if ctx.Err() != nil {
			return lastRead, ctx.Err()
		}
		msgs, err := t.fetchBlockWithRetry(ctx, logger, peer, offsetID, lastRead)
		if err != nil {
			return lastRead, fmt.Errorf("failed to fetch history page: %w", err)
		}
		if len(msgs) == 0 {
			break
		}
		sort.Slice(msgs, func(i, j int) bool { return msgs[i].ID < msgs[j].ID })
		blocks = append(blocks, msgs)
		if len(msgs) < pageSize {
			break
		}
		offsetID = msgs[0].ID
		if len(blocks) >= maxCollectBlocks {
			return lastRead, fmt.Errorf("more than %d messages above the resume point; narrow the range or reset progress", maxCollectBlocks*pageSize)
		}
	}

	cursor := lastRead
	for i := len(blocks) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			return cursor, ctx.Err()
		}
		block := blocks[i]

		highest, capped, err := t.runBlock(ctx, logger, block, stats)
		if err != nil {
			t.saveCheckpoint(ctx, logger, cursor)
			return cursor, err
		}

		if highest > cursor {
			cursor = highest
		}
		t.saveCheckpoint(ctx, logger, cursor)

		taskevent.Emit(ctx, taskevent.Event{
			TaskID:          t.ID,
			Phase:           taskevent.PhaseProgress,
			DownloadedFiles: int(stats.saved.Load()),
		})

		if capped {
			break
		}
		if t.Params.MaxMessages > 0 && stats.saved.Load() >= int64(t.Params.MaxMessages) {
			break
		}
	}
	return cursor, nil
}

// runBlock processes one history page through the batch file engine. It
// returns the highest message ID seen in the block and whether processing was
// cut short by the MaxMessages cap (messages beyond the cap stay re-fetchable
// above the returned ID).
//
// Checkpoint semantics match telegram_media_downloader: the caller advances
// its cursor across the whole block; individual failed downloads are recorded
// in the failed-ID set (persisted) and retried explicitly, so nothing is lost
// even though the cursor moves past them.
func (t *Task) runBlock(ctx context.Context, logger *log.Logger, block []*tg.Message, stats *Stats) (highest int, capped bool, err error) {
	if len(block) == 0 {
		return 0, false, nil
	}
	highest = block[len(block)-1].ID

	elems := make([]batchtfile.TaskElement, 0, len(block))
	elemMsg := make(map[string]int, len(block))
	lastBuiltMsgID := block[0].ID
	for _, msg := range block {
		stats.MarkScanned()
		media, mediaOK := msg.GetMedia()
		if !mediaOK || media == nil {
			continue
		}
		mt, ok := ClassifyMessageMedia(media)
		if !ok || !t.types.Contains(mt) {
			stats.MarkSkipped()
			continue
		}
		file, ferr := tfile.FromMediaMessage(media, t.UserCtx.Raw, msg, tfile.WithNameIfEmpty(
			tgutil.GenFileNameFromMessage(*msg),
		))
		if ferr != nil {
			logger.Errorf("Message[%d]: could not build file: %v", msg.ID, ferr)
			stats.MarkFailed()
			t.rememberFailed(msg.ID)
			lastBuiltMsgID = msg.ID
			continue
		}
		elem, cerr := batchtfile.NewTaskElement(t.Storage, t.StorPath, file)
		if cerr != nil {
			logger.Errorf("Message[%d]: could not prepare element: %v", msg.ID, cerr)
			stats.MarkFailed()
			t.rememberFailed(msg.ID)
			lastBuiltMsgID = msg.ID
			continue
		}
		elems = append(elems, *elem)
		elemMsg[elem.ID] = msg.ID
		lastBuiltMsgID = msg.ID
		if t.Params.MaxMessages > 0 && stats.saved.Load()+int64(len(elems)) >= int64(t.Params.MaxMessages) {
			capped = true
			highest = lastBuiltMsgID
			break
		}
	}
	if len(elems) == 0 {
		return highest, capped, nil
	}

	var tracker batchtfile.ProgressTracker
	if t.Params.ProgressChatID != 0 && t.Params.ProgressMsgID != 0 {
		tracker = batchtfile.NewProgressTracker(t.Params.ProgressMsgID, t.Params.ProgressChatID)
	}
	bt := batchtfile.NewBatchTGFileTask(t.ID, ctx, elems, tracker, true)
	execErr := bt.Execute(ctx)

	savedCount, failedCount := 0, 0
	for _, item := range bt.Items() {
		msgID, ok := elemMsg[item.ID]
		if !ok {
			continue
		}
		switch item.Phase {
		case batchtfile.ItemPhaseCompleted:
			savedCount++
			t.forgetFailed(msgID)
		case batchtfile.ItemPhaseFailed:
			failedCount++
			logger.Errorf("Message[%d]: %s failed (%s): %s",
				msgID, item.Name, failureStageName(item.FailureStage), item.Error)
			stats.MarkFailed()
			t.rememberFailed(msgID)
		default:
			// stopped/cancelled midway: neutral; stragglers are covered by
			// the failed-ID retry set on a later run if needed
		}
	}
	stats.AddSaved(savedCount)
	stats.AddFailed(failedCount)

	if execErr != nil && !errors.Is(execErr, context.Canceled) && ctx.Err() == nil {
		logger.Errorf("Batch engine reported an error for this page: %v", execErr)
	}

	// Dead-connection heuristic: an entire non-trivial page failed at once.
	if failedCount > 0 && savedCount == 0 && failedCount >= deadConnectionMinFailures && ctx.Err() == nil {
		return highest, capped, fmt.Errorf(
			"%d/%d downloads failed in one page; the connection looks down — stopping, failed files will be retried on the next run",
			failedCount, failedCount+savedCount)
	}
	return highest, capped, nil
}

func failureStageName(stage batchtfile.FailureStage) string {
	switch stage {
	case batchtfile.FailureStageDownload:
		return "download"
	case batchtfile.FailureStageCache:
		return "cache"
	case batchtfile.FailureStageUpload:
		return "upload"
	case batchtfile.FailureStageConfirm:
		return "confirm"
	case batchtfile.FailureStageBatchUpload:
		return "batch upload"
	default:
		return "internal"
	}
}

// fetchBlockWithRetry fetches up to pageSize messages older than offsetID
// but newer than minID, newest first (default, well-defined direction). It
// retries transient failures so an unstable connection does not abort the
// whole run; the checkpoint keeps processed pages safe.
func (t *Task) fetchBlockWithRetry(ctx context.Context, logger *log.Logger, peer tg.InputPeerClass, offsetID, minID int) ([]*tg.Message, error) {
	attempts := max(1, config.C().Retry)
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		result, err := t.UserCtx.Raw.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			OffsetID: offsetID,
			Limit:    pageSize,
			MinID:    minID,
		})
		if err == nil {
			return messagesFromResult(result), nil
		}
		lastErr = err
		logger.Warnf("History page fetch failed (attempt %d/%d): %v", attempt+1, attempts, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 2 * time.Second):
		}
	}
	return nil, lastErr
}

func messagesFromResult(result tg.MessagesMessagesClass) []*tg.Message {
	modified, ok := result.AsModified()
	if !ok {
		// messages.messagesNotModified or unsupported container
		return nil
	}
	raw := modified.GetMessages()
	msgs := make([]*tg.Message, 0, len(raw))
	for _, m := range raw {
		if m == nil {
			continue
		}
		if msg, ok := m.(*tg.Message); ok {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

func (t *Task) rememberFailed(id int) {
	t.failedMu.Lock()
	t.failed[id] = struct{}{}
	t.failedMu.Unlock()
}

func (t *Task) forgetFailed(id int) {
	t.failedMu.Lock()
	delete(t.failed, id)
	t.failedMu.Unlock()
}
