package msgelem

import (
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/krau/SaveAny-Bot/core/tasks/bulkdl"
	"github.com/krau/SaveAny-Bot/pkg/tcbdata"
)

func rowsFromButtons(buttons []tg.KeyboardButtonClass, perRow int) *tg.ReplyInlineMarkup {
	markup := &tg.ReplyInlineMarkup{}
	for i := 0; i < len(buttons); i += perRow {
		row := tg.KeyboardButtonRow{}
		row.Buttons = buttons[i:min(i+perRow, len(buttons))]
		markup.Rows = append(markup.Rows, row)
	}
	return markup
}

// BuildBulkDLSelectTypesKeyboard builds the first wizard step: choosing which
// media types to download. dataid references the cached tcbdata.Add holding
// the channel info.
func BuildBulkDLSelectTypesKeyboard(dataid string) (*tg.ReplyInlineMarkup, error) {
	buttons := make([]tg.KeyboardButtonClass, 0, len(bulkdl.AllMediaTypes())+1)
	allBtn := &tg.KeyboardButtonCallback{
		Text: "all",
		Data: fmt.Appendf(nil, "%s type %s all", tcbdata.TypeBulkDL, dataid),
	}
	buttons = append(buttons, allBtn)
	for _, mt := range bulkdl.AllMediaTypes() {
		buttons = append(buttons, &tg.KeyboardButtonCallback{
			Text: string(mt),
			Data: fmt.Appendf(nil, "%s type %s %s", tcbdata.TypeBulkDL, dataid, mt),
		})
	}
	return rowsFromButtons(buttons, 3), nil
}

// BuildBulkDLSelectMaxKeyboard builds the second wizard step: capping how many
// files are downloaded in this run (0 = no limit).
func BuildBulkDLSelectMaxKeyboard(dataid string) (*tg.ReplyInlineMarkup, error) {
	options := []int{0, 50, 100, 500, 1000}
	buttons := make([]tg.KeyboardButtonClass, 0, len(options))
	for _, n := range options {
		label := fmt.Sprintf("%d", n)
		if n == 0 {
			label = "∞"
		}
		buttons = append(buttons, &tg.KeyboardButtonCallback{
			Text: label,
			Data: fmt.Appendf(nil, "%s max %s %d", tcbdata.TypeBulkDL, dataid, n),
		})
	}
	return rowsFromButtons(buttons, len(options)), nil
}
