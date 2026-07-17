package keyboards

import (
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MediaLangOption is one audio-track choice shown to the superadmin.
type MediaLangOption struct {
	Code string
	Name string
}

// BuildMediaQualityKeyboard renders quality buttons (two per row) plus a
// "best" and cancel option. Callback data: "mq:<token>:<height>" ("0" = best).
func BuildMediaQualityKeyboard(token string, heights []int) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	for _, h := range heights {
		btn := tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("%dp", h),
			fmt.Sprintf("mq:%s:%d", token, h),
		)
		row = append(row, btn)
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("⭐ Eng yaxshi sifat", "mq:"+token+":0"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Bekor qilish", "mx:"+token),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// BuildMediaLangKeyboard renders audio-language buttons plus a default and
// cancel option. Callback data: "ml:<token>:<code>" (empty code = default).
func BuildMediaLangKeyboard(token string, langs []MediaLangOption) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton

	for _, l := range langs {
		label := l.Name
		if label == "" {
			label = l.Code
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(
			"🔊 "+label,
			fmt.Sprintf("ml:%s:%s", token, l.Code),
		)
		row = append(row, btn)
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("🔉 Standart audio", "ml:"+token+":"),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Bekor qilish", "mx:"+token),
	))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
