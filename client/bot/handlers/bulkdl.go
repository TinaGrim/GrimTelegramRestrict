package handlers

import (
	"strconv"
	"strings"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/ext"
	"github.com/charmbracelet/log"
	"github.com/duke-git/lancet/v2/validator"
	"github.com/gotd/td/tg"

	"github.com/krau/SaveAny-Bot/client/bot/handlers/utils/msgelem"
	"github.com/krau/SaveAny-Bot/client/bot/handlers/utils/shortcut"
	userclient "github.com/krau/SaveAny-Bot/client/user"
	"github.com/krau/SaveAny-Bot/common/i18n"
	"github.com/krau/SaveAny-Bot/common/i18n/i18nk"
	"github.com/krau/SaveAny-Bot/common/utils/tgutil"
	"github.com/krau/SaveAny-Bot/config"
	bulkdltask "github.com/krau/SaveAny-Bot/core/tasks/bulkdl"
	"github.com/krau/SaveAny-Bot/database"
	"github.com/krau/SaveAny-Bot/pkg/enums/tasktype"
	"github.com/krau/SaveAny-Bot/pkg/tcbdata"
	"github.com/krau/SaveAny-Bot/storage"
)

func userbotAvailable() bool {
	return config.C().Telegram.Userbot.Enable && userclient.GetCtx() != nil
}

// bulkdlAllTypesArg is the command token selecting every media type.
const bulkdlAllTypesArg = "all"

func handleBulkDlCmd(ctx *ext.Context, update *ext.Update) error {
	logger := log.FromContext(ctx)
	args := strings.Fields(update.EffectiveMessage.Text)
	if len(args) < 2 {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlUsage)), nil)
		return dispatcher.EndGroups
	}
	if !userbotAvailable() {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlErrorUserbotNotEnabled)), nil)
		return dispatcher.EndGroups
	}

	if args[1] == "reset" {
		return handleBulkDLReset(ctx, update, args[2:])
	}

	chatArg := args[1]

	typesInput, maxMessages, err := bulkdltask.ParseCommandArgs(args[2:])
	if err != nil {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlUsage)), nil)
		return dispatcher.EndGroups
	}

	tctx := ctx
	uctx := userclient.GetCtx()
	if uctx != nil && validator.IsIntStr(chatArg) {
		// numeric IDs (especially bot-API style -100 prefixed ones) must be
		// resolved through the user client's peer storage
		tctx = uctx
	}
	chatID, err := tgutil.ParseChatID(tctx, chatArg)
	if err != nil {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgCommonErrorInvalidIdOrUsername, map[string]any{
			"Error": err.Error(),
		})), nil)
		return dispatcher.EndGroups
	}

	// Fail fast when the userbot cannot actually access the chat (not a
	// member / unknown ID) instead of queueing a task that would die later.
	if _, err := uctx.ResolveInputPeerById(chatID); err != nil {
		logger.Debugf("bulkdl chat pre-check failed: %s", err)
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlErrorChatUnreachable, map[string]any{
			"Chat":  chatArg,
			"Error": err.Error(),
		})), nil)
		return dispatcher.EndGroups
	}

	addData := tcbdata.Add{
		TaskType:      tasktype.TaskTypeBulkdl,
		BulkChatID:    chatID,
		BulkChatLabel: chatArg,
	}

	// Bare "/bulkdl <chat>": start the step-by-step wizard (types -> max
	// files -> storage). With explicit types/max given, jump straight to the
	// storage selection like before.
	if typesInput == "" && len(args) == 2 {
		dataid, err := shortcut.SetCallbackData(addData)
		if err != nil {
			logger.Errorf("Failed to cache callback data: %s", err)
			return err
		}
		markup, err := msgelem.BuildBulkDLSelectTypesKeyboard(dataid)
		if err != nil {
			logger.Errorf("Failed to build type selection keyboard: %s", err)
			return err
		}
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlPromptSelectTypes, map[string]any{
			"Chat": chatArg,
		})), &ext.ReplyOpts{Markup: markup})
		return dispatcher.EndGroups
	}

	types, err := bulkdltask.ParseMediaTypes(typesInput)
	if err != nil {
		available := bulkdltask.AllMediaTypes()
		availableNames := make([]string, 0, len(available))
		for _, t := range available {
			availableNames = append(availableNames, string(t))
		}
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlErrorInvalidMediaTypes, map[string]any{
			"Types":     typesInput,
			"Available": strings.Join(availableNames, ", "),
		})), nil)
		return dispatcher.EndGroups
	}
	typeNames := make([]string, 0, len(types))
	for _, t := range types {
		typeNames = append(typeNames, string(t))
	}

	// When every supported type is selected, show the compact "all" token
	// (which is also the accepted command argument) instead of a long list.
	typesDisplay := strings.Join(typeNames, ", ")
	if len(types) == len(bulkdltask.AllMediaTypes()) {
		typesDisplay = bulkdlAllTypesArg
	}
	addData.BulkMediaTypes = typesDisplay
	addData.BulkMaxMessages = maxMessages

	stors := storage.GetUserStorages(ctx, update.GetUserChat().GetID())
	markup, err := msgelem.BuildAddSelectStorageKeyboard(stors, addData)
	if err != nil {
		logger.Errorf("Failed to build storage selection keyboard: %s", err)
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgCommonErrorBuildStorageSelectKeyboardFailed, map[string]any{
			"Error": err.Error(),
		})), nil)
		return dispatcher.EndGroups
	}

	limitLine := ""
	if maxMessages > 0 {
		limitLine = i18n.T(i18nk.BotMsgBulkdlInfoMaxFiles, map[string]any{"Count": maxMessages}) + "\n"
	}
	ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlInfoSelectStorage, map[string]any{
		"Chat":  chatArg,
		"Types": typesDisplay,
		"Limit": limitLine,
	})), &ext.ReplyOpts{
		Markup: markup,
	})
	return dispatcher.EndGroups
}

// handleBulkDLReset clears the resume checkpoint for a channel so the next
// run starts from the very beginning of its history.
func handleBulkDLReset(ctx *ext.Context, update *ext.Update, args []string) error {
	if len(args) < 1 {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlUsage)), nil)
		return dispatcher.EndGroups
	}
	chatArg := args[0]

	tctx := ctx
	if uctx := userclient.GetCtx(); uctx != nil && validator.IsIntStr(chatArg) {
		tctx = uctx
	}
	chatID, err := tgutil.ParseChatID(tctx, chatArg)
	if err != nil {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgCommonErrorInvalidIdOrUsername, map[string]any{
			"Error": err.Error(),
		})), nil)
		return dispatcher.EndGroups
	}

	dbUser, err := database.GetUserByChatID(ctx, update.GetUserChat().GetID())
	if err != nil {
		return err
	}
	if err := database.DeleteBulkDLProgress(ctx, dbUser.ID, chatID); err != nil {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlErrorResetFailed, map[string]any{
			"Error": err.Error(),
		})), nil)
		return dispatcher.EndGroups
	}
	ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlInfoResetDone, map[string]any{
		"Chat": chatArg,
	})), nil)
	return dispatcher.EndGroups
}

// handleBulkDLCallback drives the /bulkdl wizard steps:
//
//	bulkdl type <dataid> <types-token>
//	bulkdl max <dataid> <n>
func handleBulkDLCallback(ctx *ext.Context, update *ext.Update) error {
	parts := strings.Split(string(update.CallbackQuery.Data), " ")
	if len(parts) != 4 || parts[0] != tcbdata.TypeBulkDL {
		return nil
	}
	step, dataid, value := parts[1], parts[2], parts[3]

	addData, err := shortcut.GetCallbackDataWithAnswer[tcbdata.Add](ctx, update, dataid)
	if err != nil {
		return err
	}
	queryID := update.CallbackQuery.GetQueryID()
	msgID := update.CallbackQuery.GetMsgID()
	userID := update.CallbackQuery.GetUserID()

	switch step {
	case "type":
		if value != bulkdlAllTypesArg {
			if _, err := bulkdltask.ParseMediaTypes(value); err != nil {
				ctx.AnswerCallback(msgelem.AlertCallbackAnswer(queryID, i18n.T(i18nk.BotMsgBulkdlErrorInvalidMediaTypes, map[string]any{
					"Types":     value,
					"Available": strings.Join(typeTokenList(), ", "),
				})))
				return dispatcher.EndGroups
			}
		}
		addData.BulkMediaTypes = value

		newDataid, err := shortcut.SetCallbackData(addData)
		if err != nil {
			log.FromContext(ctx).Errorf("Failed to cache callback data: %s", err)
			return err
		}
		markup, err := msgelem.BuildBulkDLSelectMaxKeyboard(newDataid)
		if err != nil {
			log.FromContext(ctx).Errorf("Failed to build max files keyboard: %s", err)
			return err
		}
		ctx.EditMessage(userID, &tg.MessagesEditMessageRequest{
			ID:          msgID,
			Message:     i18n.T(i18nk.BotMsgBulkdlPromptSelectMaxFiles, map[string]any{"Types": value}),
			ReplyMarkup: markup,
		})
	case "max":
		maxFiles, err := strconv.Atoi(value)
		if err != nil || maxFiles < 0 {
			return nil
		}
		addData.BulkMaxMessages = maxFiles

		stors := storage.GetUserStorages(ctx, userID)
		markup, err := msgelem.BuildAddSelectStorageKeyboard(stors, addData)
		if err != nil {
			log.FromContext(ctx).Errorf("Failed to build storage selection keyboard: %s", err)
			ctx.AnswerCallback(msgelem.AlertCallbackAnswer(queryID, i18n.T(i18nk.BotMsgCommonErrorBuildStorageSelectKeyboardFailed, map[string]any{
				"Error": err.Error(),
			})))
			return dispatcher.EndGroups
		}
		typesDisplay := addData.BulkMediaTypes
		if parsed, perr := bulkdltask.ParseMediaTypes(typesDisplay); perr == nil && len(parsed) == len(bulkdltask.AllMediaTypes()) {
			typesDisplay = bulkdlAllTypesArg
		}
		limitLine := ""
		if maxFiles > 0 {
			limitLine = i18n.T(i18nk.BotMsgBulkdlInfoMaxFiles, map[string]any{"Count": maxFiles}) + "\n"
		}
		ctx.EditMessage(userID, &tg.MessagesEditMessageRequest{
			ID: msgID,
			Message: i18n.T(i18nk.BotMsgBulkdlInfoSelectStorage, map[string]any{
				"Chat":  addData.BulkChatLabel,
				"Types": typesDisplay,
				"Limit": limitLine,
			}),
			ReplyMarkup: markup,
		})
	}
	return dispatcher.EndGroups
}

func typeTokenList() []string {
	tokens := make([]string, 0, len(bulkdltask.AllMediaTypes()))
	for _, mt := range bulkdltask.AllMediaTypes() {
		tokens = append(tokens, string(mt))
	}
	return tokens
}
