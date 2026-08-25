package handlers

import (
	"fmt"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/ext"
	"github.com/charmbracelet/log"
	"github.com/krau/SaveAny-Bot/client/bot/handlers/utils/msgelem"
	"github.com/krau/SaveAny-Bot/common/i18n"
	"github.com/krau/SaveAny-Bot/common/i18n/i18nk"
	"github.com/krau/SaveAny-Bot/config"
)

func handleHelpCmd(ctx *ext.Context, update *ext.Update) error {
	shortHash := config.GitCommit
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}
	markup, err := msgelem.BuildFeatureMenuKeyboard()
	if err != nil {
		log.FromContext(ctx).Errorf("Failed to build feature menu: %s", err)
		ctx.Reply(update, ext.ReplyTextString(fmt.Sprintf(i18n.T(i18nk.BotMsgHelpTextFmt), config.Version, shortHash)), nil)
		return dispatcher.EndGroups
	}
	menu := i18n.T(i18nk.BotMsgWizHelpPickFeature)
	text := fmt.Sprintf(i18n.T(i18nk.BotMsgHelpTextFmt), config.Version, shortHash) + "\n\n" + menu
	ctx.Reply(update, ext.ReplyTextString(text), &ext.ReplyOpts{Markup: markup})
	return dispatcher.EndGroups
}
