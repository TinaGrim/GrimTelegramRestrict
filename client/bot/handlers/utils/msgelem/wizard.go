package msgelem

import (
	"fmt"

	"github.com/gotd/td/tg"

	"github.com/krau/SaveAny-Bot/common/i18n"
	"github.com/krau/SaveAny-Bot/common/i18n/i18nk"
	"github.com/krau/SaveAny-Bot/pkg/tcbdata"
)

// BuildFeatureMenuKeyboard builds the step-by-step feature menu attached to
// /help: one button per wizard-driven feature.
func BuildFeatureMenuKeyboard() (*tg.ReplyInlineMarkup, error) {
	type entry struct {
		key     i18nk.Key
		feature string
	}
	entries := []entry{
		{i18nk.BotMsgWizButtonDl, "dl"},
		{i18nk.BotMsgWizButtonYtdlp, "ytdlp"},
		{i18nk.BotMsgWizButtonAria2, "aria2"},
		{i18nk.BotMsgWizButtonBulkdl, "bulkdl"},
		{i18nk.BotMsgWizButtonWatch, "watch"},
	}
	buttons := make([]tg.KeyboardButtonClass, 0, len(entries))
	for _, e := range entries {
		buttons = append(buttons, &tg.KeyboardButtonCallback{
			Text: i18n.T(e.key, nil),
			Data: fmt.Appendf(nil, "%s %s", tcbdata.TypeFeature, e.feature),
		})
	}
	return rowsFromButtons(buttons, 2), nil
}
