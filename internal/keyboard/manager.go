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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Manager generates and updates inline keyboards for Telegram bot interactions.
type Manager struct {
	releaseService    service.ReleaseServiceInterface
	logger            *zap.Logger
	config            *config.Config
	botAPI            telegram.BotAPI
	allMonthsKeyboard tgbotapi.InlineKeyboardMarkup
	mainMonthKeyboard tgbotapi.InlineKeyboardMarkup
	stopChan          chan struct{}
}

var _ ManagerInterface = (*Manager)(nil)

// NewKeyboardManager initializes a new Manager with required services and configuration.
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

// SetBotAPI assigns an active Telegram Bot API instance to the keyboard manager.
func (k *Manager) SetBotAPI(botAPI telegram.BotAPI) {
	k.botAPI = botAPI
}

// initKeyboards prepares all static and dynamic keyboards.
func (k *Manager) initKeyboards() {
	k.initAllMonthsKeyboard()

	k.updateMainMonthKeyboard()
}

// initAllMonthsKeyboard generates the full month selection grid.
func (k *Manager) initAllMonthsKeyboard() {
	months := []string{
		"january", "february", "march", "april", "may", "june",
		"july", "august", "september", "october", "november", "december",
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	for i := 0; i < len(months); i += 3 {
		var row []tgbotapi.InlineKeyboardButton
		for j := 0; j < 3 && i+j < len(months); j++ {
			month := months[i+j]
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(
				cases.Title(language.English).String(month),
				"month_"+month,
			))
		}
		rows = append(rows, row)
	}

	// Add "Back" button
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Back", "back_to_main"),
	))

	k.allMonthsKeyboard = tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// updateMainMonthKeyboard generates the primary row with previous, current, and next months.
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

	buttons := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData(
			cases.Title(language.English).String(months[prevMonth-1]),
			"month_"+months[prevMonth-1],
		),
		tgbotapi.NewInlineKeyboardButtonData(
			cases.Title(language.English).String(months[currentMonth-1]),
			"month_"+months[currentMonth-1],
		),
		tgbotapi.NewInlineKeyboardButtonData(
			cases.Title(language.English).String(months[nextMonth-1]),
			"month_"+months[nextMonth-1],
		),
		tgbotapi.NewInlineKeyboardButtonData("...", "show_all_months"),
	}

	k.mainMonthKeyboard = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(buttons...),
	)

	k.logger.Info("Updated main month keyboard", zap.String("current_month", months[currentMonth-1]))
}

// updateMainMonthKeyboardLoop keeps the keyboard relevant as time progresses.
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

// GetMainKeyboard retrieves the primary monthly navigation keyboard.
func (k *Manager) GetMainKeyboard() tgbotapi.InlineKeyboardMarkup {
	return k.mainMonthKeyboard
}

// GetAllMonthsKeyboard retrieves the full twelve-month navigation grid.
func (k *Manager) GetAllMonthsKeyboard() tgbotapi.InlineKeyboardMarkup {
	return k.allMonthsKeyboard
}

// HandleCallbackQuery processes incoming Telegram inline button clicks.
func (k *Manager) HandleCallbackQuery(ctx context.Context, callback *tgbotapi.CallbackQuery) error {
	data := callback.Data
	chatID := callback.Message.Chat.ID

	k.logger.Debug("Received callback query", zap.String("data", data), zap.Int64("chat_id", chatID))

	if strings.HasPrefix(data, "month_") {
		return k.handleMonthCallback(ctx, callback)
	}

	if data == "show_all_months" {
		return k.handleShowAllMonthsCallback(callback)
	}

	if data == "back_to_main" {
		return k.handleBackToMainCallback(callback)
	}

	k.logger.Warn("Unknown callback query", zap.String("data", data))
	return fmt.Errorf("unknown callback query: %s", data)
}

// handleMonthCallback responds to specific month selection.
func (k *Manager) handleMonthCallback(ctx context.Context, callback *tgbotapi.CallbackQuery) error {
	data := callback.Data
	chatID := callback.Message.Chat.ID

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

	msg := tgbotapi.NewMessage(chatID, response)
	msg.ReplyMarkup = k.GetMainKeyboard()

	if k.botAPI != nil {
		err := k.botAPI.SendMessageWithMarkup(chatID, response, msg.ReplyMarkup)
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
func (k *Manager) handleShowAllMonthsCallback(callback *tgbotapi.CallbackQuery) error {
	chatID := callback.Message.Chat.ID
	messageID := callback.Message.MessageID

	k.logger.Debug("Showing all months keyboard")

	if k.botAPI != nil {
		err := k.botAPI.EditMessageReplyMarkup(chatID, messageID, k.GetAllMonthsKeyboard())
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
func (k *Manager) handleBackToMainCallback(callback *tgbotapi.CallbackQuery) error {
	chatID := callback.Message.Chat.ID
	messageID := callback.Message.MessageID

	k.logger.Debug("Returning to main keyboard")

	if k.botAPI != nil {
		err := k.botAPI.EditMessageReplyMarkup(chatID, messageID, k.GetMainKeyboard())
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
