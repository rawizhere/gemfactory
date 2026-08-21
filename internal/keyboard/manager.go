// Package keyboard provides inline keyboard construction and update routines for Telegram.
package keyboard

import (
	"context"
	"fmt"
	"gemfactory/internal/config"
	"gemfactory/internal/service"
	"gemfactory/internal/telegram"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Manager handles the generation and lifecycle of month navigation inline keyboards.
type Manager struct {
	releaseService    *service.ReleaseService
	logger            *zap.Logger
	config            *config.Config
	tg                *telegram.Client
	allMonthsKeyboard telego.InlineKeyboardMarkup
	mainMonthKeyboard telego.InlineKeyboardMarkup
	lastMonth         int
	mu                sync.RWMutex
	stopChan          chan struct{}
}

// NewManager initializes keyboards and starts the periodic update ticker.
func NewManager(releaseService *service.ReleaseService, config *config.Config, logger *zap.Logger) *Manager {
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

// SetTelegramClient assigns the Telegram client instance for sending responses.
func (k *Manager) SetTelegramClient(tg *telegram.Client) {
	k.tg = tg
}

func (k *Manager) initKeyboards() {
	k.initAllMonthsKeyboard()
	k.updateMainMonthKeyboard()
}

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

	k.mu.Lock()
	k.lastMonth = currentMonth
	k.mainMonthKeyboard = *telegoutil.InlineKeyboard(telegoutil.InlineKeyboardRow(buttons...))
	k.mu.Unlock()

	k.logger.Info("Updated main month keyboard", zap.String("current_month", months[currentMonth-1]))
}

func (k *Manager) updateMainMonthKeyboardLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			k.updateMainMonthKeyboard()
		case <-k.stopChan:
			return
		}
	}
}

// GetMainKeyboard returns a pointer to the thread-safe cached 3-month keyboard.
func (k *Manager) GetMainKeyboard() *telego.InlineKeyboardMarkup {
	k.mu.RLock()
	defer k.mu.RUnlock()
	kb := k.mainMonthKeyboard
	return &kb
}

// GetAllMonthsKeyboard returns the full 12-month selection keyboard.
func (k *Manager) GetAllMonthsKeyboard() *telego.InlineKeyboardMarkup {
	return &k.allMonthsKeyboard
}

// HandleCallbackQuery dispatches button clicks to their respective handler functions.
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

func (k *Manager) handleMonthCallback(ctx context.Context, callback *telego.CallbackQuery) error {
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

	if k.tg != nil {
		return k.tg.SendMessageWithMarkup(ctx, chatID, response, k.GetMainKeyboard())
	}
	return nil
}

func (k *Manager) handleShowAllMonthsCallback(ctx context.Context, callback *telego.CallbackQuery) error {
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID
	messageID := callback.Message.GetMessageID()

	if k.tg != nil {
		return k.tg.EditMessageReplyMarkup(ctx, chatID, messageID, k.GetAllMonthsKeyboard())
	}
	return nil
}

func (k *Manager) handleBackToMainCallback(ctx context.Context, callback *telego.CallbackQuery) error {
	if callback.Message == nil {
		return fmt.Errorf("callback query message is nil")
	}
	chatID := callback.Message.GetChat().ID
	messageID := callback.Message.GetMessageID()

	if k.tg != nil {
		return k.tg.EditMessageReplyMarkup(ctx, chatID, messageID, k.GetMainKeyboard())
	}
	return nil
}

// Stop terminates the background keyboard update loop.
func (k *Manager) Stop() {
	select {
	case <-k.stopChan:
	default:
		close(k.stopChan)
	}
}
