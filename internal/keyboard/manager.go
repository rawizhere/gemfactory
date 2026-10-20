package keyboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"

	"gemfactory/internal/config"
	"gemfactory/internal/model"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"
)

type Manager struct {
	releaseService    *service.ReleaseService
	logger            *zap.Logger
	config            *config.Config
	tg                *telegram.Client
	allMonthsKeyboard telego.InlineKeyboardMarkup
	mainMonthKeyboard telego.InlineKeyboardMarkup
	lastMonth         int
}

func NewManager(releaseService *service.ReleaseService, tg *telegram.Client, config *config.Config, logger *zap.Logger) *Manager {
	k := &Manager{
		releaseService: releaseService,
		tg:             tg,
		logger:         logger,
		config:         config,
	}

	k.initKeyboards()

	return k
}

func (k *Manager) initKeyboards() {
	k.initAllMonthsKeyboard()
	k.updateMainMonthKeyboard()
}

// monthButton is a "January" button with "month_january" callback data.
func monthButton(m time.Month) telego.InlineKeyboardButton {
	return telegoutil.InlineKeyboardButton(m.String()).WithCallbackData("month_" + strings.ToLower(m.String()))
}

func (k *Manager) initAllMonthsKeyboard() {
	var rows [][]telego.InlineKeyboardButton
	for i := 0; i < len(model.Months); i += 3 {
		var row []telego.InlineKeyboardButton
		for j := 0; j < 3 && i+j < len(model.Months); j++ {
			row = append(row, monthButton(time.Month(i+j+1)))
		}
		rows = append(rows, row)
	}

	rows = append(rows, telegoutil.InlineKeyboardRow(
		telegoutil.InlineKeyboardButton("Back").WithCallbackData("back_to_main"),
	))

	k.allMonthsKeyboard = *telegoutil.InlineKeyboard(rows...)
}

func (k *Manager) updateMainMonthKeyboard() {
	loc, err := time.LoadLocation(k.config.Timezone)
	if err != nil {
		k.logger.Error("Failed to load timezone", zap.String("timezone", k.config.Timezone), zap.Error(err))
		loc = time.UTC
	}

	currentMonth := int(time.Now().In(loc).Month())
	if currentMonth == k.lastMonth {
		return
	}

	cur := time.Month(currentMonth)
	buttons := []telego.InlineKeyboardButton{
		monthButton((cur+10)%12 + 1), // previous month
		monthButton(cur),
		monthButton(cur%12 + 1), // next month
		telegoutil.InlineKeyboardButton("...").WithCallbackData("show_all_months"),
	}

	k.lastMonth = currentMonth
	k.mainMonthKeyboard = *telegoutil.InlineKeyboard(telegoutil.InlineKeyboardRow(buttons...))

	k.logger.Info("Updated main month keyboard", zap.String("current_month", model.Months[currentMonth-1]))
}

func (k *Manager) GetMainKeyboard() *telego.InlineKeyboardMarkup {
	k.updateMainMonthKeyboard()
	kb := k.mainMonthKeyboard
	return &kb
}

func (k *Manager) GetAllMonthsKeyboard() *telego.InlineKeyboardMarkup {
	return &k.allMonthsKeyboard
}

// HandleMonthCallback shows releases for the "month_<name>" callback data.
func (k *Manager) HandleMonthCallback(ctx context.Context, callback *telego.CallbackQuery) error {
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID

	month := strings.TrimPrefix(callback.Data, "month_")
	currentYear := time.Now().Year()
	monthWithYear := fmt.Sprintf("%s-%d", month, currentYear)

	releases, err := k.releaseService.GetReleasesForMonth(ctx, monthWithYear, false, false)
	if err != nil {
		k.logger.Error("Failed to get releases for month", zap.String("month", monthWithYear), zap.Error(err))
		return fmt.Errorf("failed to get releases for month %s: %w", monthWithYear, err)
	}

	response := releases
	if response == "" {
		response = fmt.Sprintf("No releases found for %s.", month)
	}

	return k.tg.SendMessageWithMarkup(ctx, chatID, response, k.GetMainKeyboard())
}

// HandleAllMonthsCallback switches the inline keyboard to the full month grid.
func (k *Manager) HandleAllMonthsCallback(ctx context.Context, callback *telego.CallbackQuery) error {
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID
	messageID := callback.Message.GetMessageID()

	return k.tg.EditMessageReplyMarkup(ctx, chatID, messageID, k.GetAllMonthsKeyboard())
}

// HandleMainCallback restores the default inline keyboard.
func (k *Manager) HandleMainCallback(ctx context.Context, callback *telego.CallbackQuery) error {
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID
	messageID := callback.Message.GetMessageID()

	return k.tg.EditMessageReplyMarkup(ctx, chatID, messageID, k.GetMainKeyboard())
}
