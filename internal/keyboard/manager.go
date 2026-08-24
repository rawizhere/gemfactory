package keyboard

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
	"go.uber.org/zap"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

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
	mu                sync.RWMutex
	stopChan          chan struct{}
}

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

func (k *Manager) SetTelegramClient(tg *telegram.Client) {
	k.tg = tg
}

func (k *Manager) initKeyboards() {
	k.initAllMonthsKeyboard()
	k.updateMainMonthKeyboard()
}

func (k *Manager) initAllMonthsKeyboard() {
	var rows [][]telego.InlineKeyboardButton
	for i := 0; i < len(model.Months); i += 3 {
		var row []telego.InlineKeyboardButton
		for j := 0; j < 3 && i+j < len(model.Months); j++ {
			month := model.Months[i+j]
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

	buttons := []telego.InlineKeyboardButton{
		telegoutil.InlineKeyboardButton(cases.Title(language.English).String(model.Months[prevMonth-1])).
			WithCallbackData("month_" + model.Months[prevMonth-1]),
		telegoutil.InlineKeyboardButton(cases.Title(language.English).String(model.Months[currentMonth-1])).
			WithCallbackData("month_" + model.Months[currentMonth-1]),
		telegoutil.InlineKeyboardButton(cases.Title(language.English).String(model.Months[nextMonth-1])).
			WithCallbackData("month_" + model.Months[nextMonth-1]),
		telegoutil.InlineKeyboardButton("...").WithCallbackData("show_all_months"),
	}

	k.mu.Lock()
	k.lastMonth = currentMonth
	k.mainMonthKeyboard = *telegoutil.InlineKeyboard(telegoutil.InlineKeyboardRow(buttons...))
	k.mu.Unlock()

	k.logger.Info("Updated main month keyboard", zap.String("current_month", model.Months[currentMonth-1]))
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

func (k *Manager) GetMainKeyboard() *telego.InlineKeyboardMarkup {
	k.mu.RLock()
	defer k.mu.RUnlock()
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

	if k.tg != nil {
		return k.tg.SendMessageWithMarkup(ctx, chatID, response, k.GetMainKeyboard())
	}
	return nil
}

// HandleAllMonthsCallback switches the inline keyboard to the full month grid.
func (k *Manager) HandleAllMonthsCallback(ctx context.Context, callback *telego.CallbackQuery) error {
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

// HandleMainCallback restores the default inline keyboard.
func (k *Manager) HandleMainCallback(ctx context.Context, callback *telego.CallbackQuery) error {
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

func (k *Manager) Stop() {
	select {
	case <-k.stopChan:
	default:
		close(k.stopChan)
	}
}
