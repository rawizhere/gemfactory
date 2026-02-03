// Package keyboard implements the Telegram bot's inline keyboard navigation system.
package keyboard

import (
	"context"
	"fmt"
	"gemfactory/internal/config"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Manager handles inline keyboard generation and updates.
type Manager struct {
	releaseService    service.ReleaseServiceInterface
	logger            *zap.Logger
	config            *config.Config
	botAPI            telegram.BotAPI
	allMonthsKeyboard telego.InlineKeyboardMarkup
	mainMonthKeyboard telego.InlineKeyboardMarkup
	stopChan          chan struct{}
}

var _ ManagerInterface = (*Manager)(nil)

// NewKeyboardManager creates a new Manager.
func NewKeyboardManager(releaseService service.ReleaseServiceInterface, config *config.Config, logger *zap.Logger) *Manager {
	k := &Manager{
		releaseService: releaseService,
		logger:         logger,
		config:         config,
		stopChan:       make(chan struct{}),
	}

	k.initKeyboards()

	go k.updateMainMonthKeyboardLoop()

	return k
}

// SetBotAPI sets the Telegram Bot API instance.
func (k *Manager) SetBotAPI(botAPI telegram.BotAPI) {
	k.botAPI = botAPI
}

// initKeyboards prepares all static and dynamic keyboards.
func (k *Manager) initKeyboards() {
	k.initAllMonthsKeyboard()

	k.updateMainMonthKeyboard()
}

// initAllMonthsKeyboard creates the month selection grid.
func (k *Manager) initAllMonthsKeyboard() {
	months := []string{
		"january", "february", "march", "april", "may", "june",
		"july", "august", "september", "october", "november", "december",
	}

	var rows [][]telego.InlineKeyboardButton
	for i := 0; i < len(months); i += 3 {
		var row []telego.InlineKeyboardButton
		for j := 0; j < 3 && i+j < len(months); j++ {
			month := months[i+j]
			row = append(row, telegoutil.InlineKeyboardButton(cases.Title(language.English).String(month)).
				WithCallbackData("month_"+month))
		}
		rows = append(rows, row)
	}

	// Add "Back" button
	rows = append(rows, telegoutil.InlineKeyboardRow(
		telegoutil.InlineKeyboardButton("Back").WithCallbackData("back_to_main"),
	))

	k.allMonthsKeyboard = *telegoutil.InlineKeyboard(rows...)
}

// updateMainMonthKeyboard creates the primary month selection row.
func (k *Manager) updateMainMonthKeyboard() {
	loc, err := time.LoadLocation(k.config.Timezone)
	if err != nil {
		k.logger.Error("Failed to load timezone", zap.String("timezone", k.config.Timezone), zap.Error(err))
		loc = time.UTC
	}

	currentMonth := int(time.Now().In(loc).Month())
	prevMonth := currentMonth - 1
	if prevMonth < 1 {
		prevMonth = 12
	}
	nextMonth := currentMonth + 1
	if nextMonth > 12 {
		nextMonth = 1
	}

	months := []string{
		"january", "february", "march", "april", "may", "june",
		"july", "august", "september", "october", "november", "december",
	}

	buttons := []telego.InlineKeyboardButton{
		telegoutil.InlineKeyboardButton(cases.Title(language.English).String(months[prevMonth-1])).
			WithCallbackData("month_" + months[prevMonth-1]),
		telegoutil.InlineKeyboardButton(cases.Title(language.English).String(months[currentMonth-1])).
			WithCallbackData("month_" + months[currentMonth-1]),
		telegoutil.InlineKeyboardButton(cases.Title(language.English).String(months[nextMonth-1])).
			WithCallbackData("month_" + months[nextMonth-1]),
		telegoutil.InlineKeyboardButton("...").WithCallbackData("show_all_months"),
	}

	k.mainMonthKeyboard = *telegoutil.InlineKeyboard(telegoutil.InlineKeyboardRow(buttons...))

	k.logger.Info("Updated main month keyboard", zap.String("current_month", months[currentMonth-1]))
}

// updateMainMonthKeyboardLoop updates the keyboard as time progresses.
func (k *Manager) updateMainMonthKeyboardLoop() {
	for {
		select {
		case <-k.stopChan:
			return
		default:
			loc, err := time.LoadLocation(k.config.Timezone)
			if err != nil {
				k.logger.Error("Failed to load timezone", zap.String("timezone", k.config.Timezone), zap.Error(err))
				loc = time.UTC
			}

			now := time.Now().In(loc)
			nextMonth := now.AddDate(0, 1, 0)
			firstOfNextMonth := time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, loc)
			durationUntilFirst := firstOfNextMonth.Sub(now)

			select {
			case <-time.After(durationUntilFirst):
				k.updateMainMonthKeyboard()
			case <-k.stopChan:
				return
			}
		}
	}
}

// GetMainKeyboard returns the primary navigation keyboard.
func (k *Manager) GetMainKeyboard() *telego.InlineKeyboardMarkup {
	return &k.mainMonthKeyboard
}

// GetAllMonthsKeyboard retrieves the full twelve-month navigation grid.
func (k *Manager) GetAllMonthsKeyboard() *telego.InlineKeyboardMarkup {
	return &k.allMonthsKeyboard
}

// HandleCallbackQuery processes incoming Telegram inline button clicks.
func (k *Manager) HandleCallbackQuery(ctx context.Context, callback *telego.CallbackQuery) error {
	data := callback.Data
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID

	k.logger.Debug("Received callback query", zap.String("data", data), zap.Int64("chat_id", chatID))

	if strings.HasPrefix(data, "month_") {
		return k.handleMonthCallback(ctx, callback)
	}

	if data == "show_all_months" {
		return k.handleShowAllMonthsCallback(ctx, callback)
	}

	if data == "back_to_main" {
		return k.handleBackToMainCallback(ctx, callback)
	}

	k.logger.Warn("Unknown callback query", zap.String("data", data))
	return fmt.Errorf("unknown callback query: %s", data)
}

// handleMonthCallback responds to specific month selection.
func (k *Manager) handleMonthCallback(ctx context.Context, callback *telego.CallbackQuery) error {
	data := callback.Data
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID

	month := strings.TrimPrefix(data, "month_")
	currentYear := time.Now().Year()

	monthWithYear := fmt.Sprintf("%s-%d", month, currentYear)

	k.logger.Debug("Processing month callback",
		zap.String("month", month),
		zap.Int("year", currentYear),
		zap.String("month_with_year", monthWithYear))

	releases, err := k.releaseService.GetReleasesForMonth(ctx, monthWithYear, false, false)
	if err != nil {
		k.logger.Error("Failed to get releases for month", zap.String("month", monthWithYear), zap.Error(err))
		return fmt.Errorf("failed to get releases for month %s: %w", monthWithYear, err)
	}

	response := releases // Assuming releases is now the string response or needs formatting
	if response == "" {
		k.logger.Warn("Empty response for month", zap.String("month", month))
		response = fmt.Sprintf("No releases found for %s.", month)
	}

	if k.botAPI != nil {
		err := k.botAPI.SendMessageWithMarkup(ctx, chatID, response, k.GetMainKeyboard())
		if err != nil {
			k.logger.Error("Failed to send message with markup", zap.Int64("chat_id", chatID), zap.Error(err))
			return err
		}
	} else {
		k.logger.Warn("BotAPI not available, cannot send message", zap.Int64("chat_id", chatID))
	}

	return nil
}

// handleShowAllMonthsCallback replaces the current keyboard with the full month grid.
func (k *Manager) handleShowAllMonthsCallback(ctx context.Context, callback *telego.CallbackQuery) error {
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID
	messageID := callback.Message.GetMessageID()

	k.logger.Debug("Showing all months keyboard")

	if k.botAPI != nil {
		err := k.botAPI.EditMessageReplyMarkup(ctx, chatID, messageID, k.GetAllMonthsKeyboard())
		if err != nil {
			k.logger.Error("Failed to edit message markup", zap.Int64("chat_id", chatID), zap.Error(err))
			return err
		}
	} else {
		k.logger.Warn("BotAPI not available, cannot edit message", zap.Int64("chat_id", chatID))
	}

	return nil
}

// handleBackToMainCallback restores the primary monthly keyboard.
func (k *Manager) handleBackToMainCallback(ctx context.Context, callback *telego.CallbackQuery) error {
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID
	messageID := callback.Message.GetMessageID()

	k.logger.Debug("Returning to main keyboard")

	if k.botAPI != nil {
		err := k.botAPI.EditMessageReplyMarkup(ctx, chatID, messageID, k.GetMainKeyboard())
		if err != nil {
			k.logger.Error("Failed to edit message markup", zap.Int64("chat_id", chatID), zap.Error(err))
			return err
		}
	} else {
		k.logger.Warn("BotAPI not available, cannot edit message", zap.Int64("chat_id", chatID))
	}

	return nil
}

// Stop halts the background update loop.
func (k *Manager) Stop() {
	close(k.stopChan)
}
