package shortcut

import (
	"fmt"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/ext"
	"github.com/charmbracelet/log"
	"github.com/gotd/td/tg"
	"github.com/rs/xid"

	userclient "github.com/krau/SaveAny-Bot/client/user"
	"github.com/krau/SaveAny-Bot/common/i18n"
	"github.com/krau/SaveAny-Bot/common/i18n/i18nk"
	"github.com/krau/SaveAny-Bot/common/utils/tgutil"
	"github.com/krau/SaveAny-Bot/config"
	"github.com/krau/SaveAny-Bot/core"
	bulkdltask "github.com/krau/SaveAny-Bot/core/tasks/bulkdl"
	"github.com/krau/SaveAny-Bot/database"
	"github.com/krau/SaveAny-Bot/storage"
)

// CreateAndAddBulkDLTaskWithEdit creates a bulk channel download task and
// edits the storage-selection message to confirm.
func CreateAndAddBulkDLTaskWithEdit(
	ctx *ext.Context,
	userChatID int64,
	selectedStorage storage.Storage,
	dirPath string,
	chatID int64,
	chatLabel string,
	mediaTypes string,
	maxMessages int,
	msgID int,
) error {
	logger := log.FromContext(ctx)
	injectCtx := tgutil.ExtWithContext(ctx.Context, ctx)

	if !config.C().Telegram.Userbot.Enable || userclient.GetCtx() == nil {
		ctx.EditMessage(userChatID, &tg.MessagesEditMessageRequest{
			ID:      msgID,
			Message: i18n.T(i18nk.BotMsgBulkdlErrorUserbotNotEnabled, nil),
		})
		return dispatcher.EndGroups
	}

	dbUser, err := database.GetUserByChatID(ctx, userChatID)
	if err != nil {
		logger.Errorf("Failed to get user: %s", err)
		ctx.EditMessage(userChatID, &tg.MessagesEditMessageRequest{
			ID:      msgID,
			Message: i18n.T(i18nk.BotMsgCommonErrorTaskCreateFailed, map[string]any{"Error": err.Error()}),
		})
		return dispatcher.EndGroups
	}

	types, err := bulkdltask.ParseMediaTypes(mediaTypes)
	if err != nil {
		ctx.EditMessage(userChatID, &tg.MessagesEditMessageRequest{
			ID:      msgID,
			Message: i18n.T(i18nk.BotMsgBulkdlErrorInvalidMediaTypes, map[string]any{"Types": mediaTypes}),
		})
		return dispatcher.EndGroups
	}

	task := bulkdltask.NewTask(
		xid.New().String(),
		injectCtx,
		userclient.GetCtx(),
		bulkdltask.Params{
			UserID:         dbUser.ID,
			ChatID:         chatID,
			ChatLabel:      chatLabel,
			Types:          types,
			MaxMessages:    maxMessages,
			ProgressMsgID:  msgID,
			ProgressChatID: userChatID,
		},
		selectedStorage,
		dirPath,
		bulkdltask.NewProgress(msgID, userChatID),
	)
	if err := core.AddTask(injectCtx, task); err != nil {
		logger.Errorf("Failed to add bulkdl task: %s", err)
		ctx.EditMessage(userChatID, &tg.MessagesEditMessageRequest{
			ID:      msgID,
			Message: i18n.T(i18nk.BotMsgCommonErrorTaskAddFailed, map[string]any{"Error": err.Error()}),
		})
		return dispatcher.EndGroups
	}

	ctx.EditMessage(userChatID, &tg.MessagesEditMessageRequest{
		ID:      msgID,
		Message: fmt.Sprintf("%s\n%s", i18n.T(i18nk.BotMsgCommonInfoTaskAdded, nil), task.Title()),
	})
	return dispatcher.EndGroups
}
