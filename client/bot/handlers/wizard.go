package handlers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/ext"
	"github.com/charmbracelet/log"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/gotd/td/tg"

	"github.com/krau/SaveAny-Bot/client/bot/handlers/utils/msgelem"
	"github.com/krau/SaveAny-Bot/client/bot/handlers/utils/shortcut"
	userclient "github.com/krau/SaveAny-Bot/client/user"
	"github.com/krau/SaveAny-Bot/common/cache"
	"github.com/krau/SaveAny-Bot/common/i18n"
	"github.com/krau/SaveAny-Bot/common/i18n/i18nk"
	"github.com/krau/SaveAny-Bot/common/utils/tgutil"
	"github.com/krau/SaveAny-Bot/config"
	bulkdltask "github.com/krau/SaveAny-Bot/core/tasks/bulkdl"
	"github.com/krau/SaveAny-Bot/pkg/enums/tasktype"
	"github.com/krau/SaveAny-Bot/pkg/tcbdata"
	"github.com/krau/SaveAny-Bot/storage"
)

// Step-by-step wizards: every feature can be driven purely with buttons plus
// one plain-text reply, without memorising command syntax.
//
// Flow: /help menu -> tap feature button ("feat <name>") -> bot asks for the
// required input -> user sends it -> existing per-feature pipeline takes over
// (storage selection keyboards, bulkdl type/max wizard, ...).

const (
	wizCacheKeyPrefix = "wiz:"
	wizFeatDl         = "dl"
	wizFeatYtdlp      = "ytdlp"
	wizFeatAria2      = "aria2"
	wizFeatBulkdl     = "bulkdl"
	wizFeatWatch      = "watch"
)

func wizCacheKey(userID int64) string {
	return fmt.Sprintf("%s%d", wizCacheKeyPrefix, userID)
}

func setAwaitingFeature(userID int64, feature string) error {
	return cache.Set(wizCacheKey(userID), feature)
}

func takeAwaitingFeature(userID int64) (string, bool) {
	feature, ok := cache.Get[string](wizCacheKey(userID))
	if !ok {
		return "", false
	}
	cache.Delete(wizCacheKey(userID))
	return feature, true
}

// handleWizardInput consumes plain-text messages while the user is inside a
// wizard. Registered before every other text handler; texts starting with "/"
// are ignored so commands keep working mid-wizard.
func handleWizardInput(ctx *ext.Context, update *ext.Update) error {
	text := strings.TrimSpace(update.EffectiveMessage.Text)
	if strings.HasPrefix(text, "/") {
		// Any explicit command cancels a pending wizard step.
		cache.Delete(wizCacheKey(update.GetUserChat().GetID()))
		return nil // let the normal command chain run
	}
	feature, ok := takeAwaitingFeature(update.GetUserChat().GetID())
	if !ok {
		return nil // not in a wizard; normal chain handles it
	}
	logger := log.FromContext(ctx)
	logger.Debugf("wizard input for %s: %q", feature, text)

	switch feature {
	case wizFeatDl:
		return wizardContinueLinks(ctx, update, text, tasktype.TaskTypeDirectlinks)
	case wizFeatAria2:
		return wizardContinueAria2(ctx, update, text)
	case wizFeatYtdlp:
		return wizardContinueYtdlp(ctx, update, text)
	case wizFeatBulkdl:
		return wizardContinueBulkdl(ctx, update, text)
	case wizFeatWatch:
		return startWatching(ctx, update, text)
	default:
		return nil
	}
}

// handleFeatureButton starts a wizard from the /help feature menu.
func handleFeatureButton(ctx *ext.Context, update *ext.Update) error {
	parts := strings.Split(string(update.CallbackQuery.Data), " ")
	if len(parts) != 2 || parts[0] != tcbdata.TypeFeature {
		return nil
	}
	feature := parts[1]
	userID := update.CallbackQuery.GetUserID()

	promptKey := i18nk.Key("")
	switch feature {
	case wizFeatDl:
		promptKey = i18nk.BotMsgWizPromptDl
	case wizFeatYtdlp:
		promptKey = i18nk.BotMsgWizPromptYtdlp
	case wizFeatAria2:
		promptKey = i18nk.BotMsgWizPromptAria2
	case wizFeatBulkdl:
		promptKey = i18nk.BotMsgWizPromptBulkdl
	case wizFeatWatch:
		promptKey = i18nk.BotMsgWizPromptWatch
	default:
		return nil
	}

	if err := setAwaitingFeature(userID, feature); err != nil {
		log.FromContext(ctx).Errorf("Failed to store wizard state: %s", err)
		return err
	}
	ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{})
	ctx.EditMessage(userID, &tg.MessagesEditMessageRequest{
		ID:      update.CallbackQuery.GetMsgID(),
		Message: i18n.T(promptKey),
	})
	return dispatcher.EndGroups
}

// --- continuations: turn the captured text into each feature's flow ---

func validURLs(text string) []string {
	links := strings.Fields(text)
	for i, link := range links {
		u, err := url.Parse(link)
		if err != nil || u.Scheme == "" || u.Host == "" {
			links[i] = ""
		}
	}
	return slice.Compact(links)
}

func wizardContinueLinks(ctx *ext.Context, update *ext.Update, text string, tt tasktype.TaskType) error {
	links := validURLs(text)
	if len(links) == 0 {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgDlErrorNoValidLinks)), nil)
		return dispatcher.EndGroups
	}
	markup, err := msgelem.BuildAddSelectStorageKeyboard(storage.GetUserStorages(ctx, update.GetUserChat().GetID()), tcbdata.Add{
		TaskType:    tt,
		DirectLinks: links,
	})
	if err != nil {
		return err
	}
	ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgDlInfoFilesSelectStorage, map[string]any{
		"Count": len(links),
	})), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

func wizardContinueAria2(ctx *ext.Context, update *ext.Update, text string) error {
	if !config.C().Aria2.Enable {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgAria2ErrorAria2NotEnabled)), nil)
		return dispatcher.EndGroups
	}
	links := slice.Compact(strings.Fields(text))
	if len(links) == 0 {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgDlErrorNoValidLinks)), nil)
		return dispatcher.EndGroups
	}
	if _, err := GetOrCreateAria2Client(); err != nil {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgAria2ErrorAria2ClientInitFailed, map[string]any{
			"Error": err.Error(),
		})), nil)
		return dispatcher.EndGroups
	}
	markup, kerr := msgelem.BuildAddSelectStorageKeyboard(storage.GetUserStorages(ctx, update.GetUserChat().GetID()), tcbdata.Add{
		TaskType:  tasktype.TaskTypeAria2,
		Aria2URIs: links,
	})
	if kerr != nil {
		return kerr
	}
	ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgAria2InfoSelectStorage)), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

func wizardContinueYtdlp(ctx *ext.Context, update *ext.Update, text string) error {
	urls := validURLs(text)
	if len(urls) == 0 {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgYtdlpErrorNoValidUrls)), nil)
		return dispatcher.EndGroups
	}
	markup, err := msgelem.BuildAddSelectStorageKeyboard(storage.GetUserStorages(ctx, update.GetUserChat().GetID()), tcbdata.Add{
		TaskType:  tasktype.TaskTypeYtdlp,
		YtdlpURLs: urls,
	})
	if err != nil {
		return err
	}
	ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgYtdlpInfoUrlsSelectStorage, map[string]any{
		"Count": len(urls),
	})), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

func wizardContinueBulkdl(ctx *ext.Context, update *ext.Update, text string) error {
	logger := log.FromContext(ctx)
	fields := strings.Fields(text)
	if len(fields) < 1 {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlUsage)), nil)
		return dispatcher.EndGroups
	}
	if !userbotAvailable() {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlErrorUserbotNotEnabled)), nil)
		return dispatcher.EndGroups
	}
	chatArg := fields[0]
	typesArg := ""
	maxMessages := 0
	if len(fields) > 1 {
		parsedTypes, parsedMax, err := bulkdltask.ParseCommandArgs(fields[1:])
		if err != nil {
			ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlUsage)), nil)
			return dispatcher.EndGroups
		}
		typesArg, maxMessages = parsedTypes, parsedMax
	}

	tctx := ctx
	if uctx := userclient.GetCtx(); uctx != nil && isNumeric(chatArg) {
		tctx = uctx
	}
	chatID, err := tgutil.ParseChatID(tctx, chatArg)
	if err != nil {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgCommonErrorInvalidIdOrUsername, map[string]any{"Error": err.Error()})), nil)
		return dispatcher.EndGroups
	}
	if _, err := userclient.GetCtx().ResolveInputPeerById(chatID); err != nil {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlErrorChatUnreachable, map[string]any{
			"Chat": chatArg, "Error": err.Error(),
		})), nil)
		return dispatcher.EndGroups
	}

	addData := tcbdata.Add{
		TaskType:      tasktype.TaskTypeBulkdl,
		BulkChatID:    chatID,
		BulkChatLabel: chatArg,
	}

	// Optional inline args skip straight to storage selection; otherwise
	// continue with the type -> max files wizard steps.
	if typesArg != "" || maxMessages > 0 || len(fields) > 2 {
		return launchBulkDLWithArgs(ctx, update, addData, typesArg, maxMessages)
	}

	dataid, err := setCallbackData(addData)
	if err != nil {
		logger.Errorf("Failed to cache callback data: %s", err)
		return err
	}
	markup, err := msgelem.BuildBulkDLSelectTypesKeyboard(dataid)
	if err != nil {
		return err
	}
	ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlPromptSelectTypes, map[string]any{
		"Chat": chatArg,
	})), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

// launchBulkDLWithArgs skips the type/max wizard when the user supplied
// inline arguments and jumps straight to storage selection.
func launchBulkDLWithArgs(ctx *ext.Context, update *ext.Update, addData tcbdata.Add, typesArg string, maxMessages int) error {
	types, err := bulkdltask.ParseMediaTypes(typesArg)
	if err != nil {
		ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlErrorInvalidMediaTypes, map[string]any{
			"Types": typesArg,
		})), nil)
		return dispatcher.EndGroups
	}
	typeNames := make([]string, 0, len(types))
	for _, mt := range types {
		typeNames = append(typeNames, string(mt))
	}
	typesDisplay := strings.Join(typeNames, ", ")
	if len(types) == len(bulkdltask.AllMediaTypes()) {
		typesDisplay = bulkdlAllTypesArg
	}
	addData.BulkMediaTypes = typesDisplay
	addData.BulkMaxMessages = maxMessages

	markup, err := msgelem.BuildAddSelectStorageKeyboard(storage.GetUserStorages(ctx, update.GetUserChat().GetID()), addData)
	if err != nil {
		return err
	}
	limitLine := ""
	if maxMessages > 0 {
		limitLine = i18n.T(i18nk.BotMsgBulkdlInfoMaxFiles, map[string]any{"Count": maxMessages}) + "\n"
	}
	ctx.Reply(update, ext.ReplyTextString(i18n.T(i18nk.BotMsgBulkdlInfoSelectStorage, map[string]any{
		"Chat":  addData.BulkChatLabel,
		"Types": typesDisplay,
		"Limit": limitLine,
	})), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}

// setCallbackData stores callback payload under a fresh data ID.
func setCallbackData[DataType any](data DataType) (string, error) {
	return shortcut.SetCallbackData(data)
}

func isNumeric(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return len(s) > 0
}
