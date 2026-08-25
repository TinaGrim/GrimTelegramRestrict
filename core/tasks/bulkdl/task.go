// Package bulkdl implements channel-wide media bulk download for SaveAny-Bot
// (https://github.com/krau/SaveAny-Bot) by krau.
//
// The download strategy, media type classification, checkpoint/resume and
// failed-retry semantics are ported from telegram_media_downloader
// (https://github.com/Dineshkarthik/telegram_media_downloader)
// by Dineshkarthik (AGPL-3.0), in accordance with SaveAny-Bot's AGPL-3.0
// license.
package bulkdl

import (
	"context"
	"fmt"
	"sync"

	"github.com/celestix/gotgproto/ext"
	"github.com/krau/SaveAny-Bot/core"
	"github.com/krau/SaveAny-Bot/pkg/enums/tasktype"
	"github.com/krau/SaveAny-Bot/storage"
)

var _ core.Executable = (*Task)(nil)

// Params carries everything needed to run one bulk channel download.
type Params struct {
	// UserID is the database user owning the task; checkpoints are stored per user+chat.
	UserID uint
	// ChatID is the resolved Telegram peer ID of the source channel/chat.
	ChatID int64
	// ChatLabel is a human readable reference (username or raw arg) for UI.
	ChatLabel string
	// Types restricts which media kinds are downloaded.
	Types []MediaType
	// MaxMessages caps how many files are downloaded in this run (0 = unlimited).
	MaxMessages int
	// ProgressMsgID/ProgressChatID point at the Telegram message used for
	// live per-file progress cards (0 disables that card; only start/done
	// summaries are rendered).
	ProgressMsgID  int
	ProgressChatID int64
}

type Task struct {
	ID       string
	ctx      context.Context //nolint:containedctx // consistent with other tasks
	UserCtx  *ext.Context    // userbot context used for history iteration and downloads
	Params   Params
	Storage  storage.Storage
	StorPath string
	Progress ProgressTracker

	types    MediaTypeSet
	failed   map[int]struct{}
	failedMu sync.Mutex
	// resume is the checkpoint loaded at execution start (0 on fresh runs);
	// exposed for progress rendering.
	resume int
}

func NewTask(
	id string,
	ctx context.Context,
	userCtx *ext.Context,
	params Params,
	stor storage.Storage,
	storPath string,
	progress ProgressTracker,
) *Task {
	return &Task{
		ID:       id,
		ctx:      ctx,
		UserCtx:  userCtx,
		Params:   params,
		Storage:  stor,
		StorPath: storPath,
		Progress: progress,
		types:    NewMediaTypeSet(params.Types),
		failed:   make(map[int]struct{}),
	}
}

func (t *Task) Type() tasktype.TaskType {
	return tasktype.TaskTypeBulkdl
}

func (t *Task) Title() string {
	return fmt.Sprintf("[%s](%s)", t.Type(), t.Params.ChatLabel)
}

func (t *Task) TaskID() string {
	return t.ID
}
