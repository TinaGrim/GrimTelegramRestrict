package bulkdl

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/charmbracelet/log"
	"github.com/gotd/td/tg"

	"github.com/krau/SaveAny-Bot/common/i18n"
	"github.com/krau/SaveAny-Bot/common/i18n/i18nk"
	"github.com/krau/SaveAny-Bot/common/utils/tgutil"
)

// Stats aggregates run counters for a bulk download task. All methods are
// safe for concurrent use. Detailed live progress (per-file bars and speeds)
// is rendered by the batch engine while a page is processing; these counters
// power the start/done summary cards.
type Stats struct {
	scanned atomic.Int64
	saved   atomic.Int64
	skipped atomic.Int64
	failed  atomic.Int64
}

func newStats() *Stats {
	return &Stats{}
}

func (s *Stats) MarkScanned() { s.scanned.Add(1) }
func (s *Stats) MarkSkipped() { s.skipped.Add(1) }
func (s *Stats) MarkFailed()  { s.failed.Add(1) }

func (s *Stats) AddSaved(n int)  { s.saved.Add(int64(n)) }
func (s *Stats) AddFailed(n int) { s.failed.Add(int64(n)) }

func (s *Stats) Saved() int64 { return s.saved.Load() }

// Snapshot is a point-in-time copy of the counters.
type Snapshot struct {
	Scanned int64
	Saved   int64
	Skipped int64
	Failed  int64
}

func (s *Stats) Snapshot() Snapshot {
	return Snapshot{
		Scanned: s.scanned.Load(),
		Saved:   s.saved.Load(),
		Skipped: s.skipped.Load(),
		Failed:  s.failed.Load(),
	}
}

// ProgressTracker receives bulk-run lifecycle updates. Live per-file progress
// inside a page is handled by the batch engine itself.
type ProgressTracker interface {
	OnStart(ctx context.Context, task *Task)
	OnDone(ctx context.Context, task *Task, err error, snap Snapshot)
}

type Progress struct {
	msgID  int
	chatID int64
}

var _ ProgressTracker = (*Progress)(nil)

func NewProgress(msgID int, chatID int64) ProgressTracker {
	return &Progress{msgID: msgID, chatID: chatID}
}

func (p *Progress) OnStart(ctx context.Context, task *Task) {
	log.FromContext(ctx).Debugf("bulkdl task started: chat=%d", task.Params.ChatID)
	p.render(ctx, i18n.T(i18nk.BotMsgProgressBulkdlStart, tgutil.EscapeHTMLTemplateData(map[string]any{
		"Chat":   task.Params.ChatLabel,
		"Dest":   destination(task),
		"Resume": task.resume,
	})))
}

func (p *Progress) OnDone(ctx context.Context, task *Task, err error, snap Snapshot) {
	logger := log.FromContext(ctx)
	ext := tgutil.ExtFromContext(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			logger.Infof("bulkdl task %s was canceled", task.TaskID())
			if ext != nil {
				ext.EditMessage(p.chatID, &tg.MessagesEditMessageRequest{
					ID:      p.msgID,
					Message: i18n.T(i18nk.BotMsgProgressTaskCanceledWithId, map[string]any{"TaskID": task.TaskID()}),
				})
			}
			return
		}
		logger.Errorf("bulkdl task %s failed: %s", task.TaskID(), err)
		if ext != nil {
			ext.EditMessage(p.chatID, &tg.MessagesEditMessageRequest{
				ID:      p.msgID,
				Message: i18n.T(i18nk.BotMsgProgressTaskFailedWithError, map[string]any{"Error": err.Error()}),
			})
		}
		return
	}
	logger.Infof("bulkdl task %s completed successfully", task.TaskID())
	// A resumed run that scanned nothing usually means either everything is
	// genuinely saved already, or the checkpoint came from the buggy
	// top-down version. Tell the user how to tell apart / fix both.
	if snap.Scanned == 0 && task.resume > 0 {
		p.render(ctx, i18n.T(i18nk.BotMsgProgressBulkdlDoneUpToDate, tgutil.EscapeHTMLTemplateData(map[string]any{
			"Chat":   task.Params.ChatLabel,
			"Resume": task.resume,
			"Dest":   destination(task),
		})))
		return
	}
	p.render(ctx, i18n.T(i18nk.BotMsgProgressBulkdlDone, tgutil.EscapeHTMLTemplateData(map[string]any{
		"Chat":    task.Params.ChatLabel,
		"Scanned": snap.Scanned,
		"Saved":   snap.Saved,
		"Skipped": snap.Skipped,
		"Failed":  snap.Failed,
		"Dest":    destination(task),
	})))
}

// destination renders the "[storage]:path" save location for progress texts.
func destination(task *Task) string {
	return fmt.Sprintf("[%s]:%s", task.Storage.Name(), task.StorPath)
}

// render edits the progress message with the given HTML markup.
func (p *Progress) render(ctx context.Context, markup string) {
	text, entities, err := tgutil.RenderHTML(markup)
	if err != nil {
		log.FromContext(ctx).Errorf("Failed to render progress markup: %s", err)
		return
	}
	req := &tg.MessagesEditMessageRequest{ID: p.msgID}
	req.SetMessage(text)
	req.SetEntities(entities)
	ext := tgutil.ExtFromContext(ctx)
	if ext == nil {
		return
	}
	ext.EditMessage(p.chatID, req)
}
