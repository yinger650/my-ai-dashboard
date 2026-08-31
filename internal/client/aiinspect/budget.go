package aiinspect

import (
	"encoding/json"
	"strconv"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/spool"
	"agentboard/internal/event"
	"agentboard/internal/shared"
)

const budgetKey = "ai.budget"

// Budget is the UTC-day call/token counter.
type Budget struct {
	Date         string            `json:"date"`
	Calls        int               `json:"calls"`
	InputTokens  int               `json:"input_tokens"`
	OutputTokens int               `json:"output_tokens"`
	Notices      map[string]string `json:"notices"`
}

func todayUTC() string { return shared.NowUTC().UTC().Format("2006-01-02") }

// LoadBudget reads or resets the daily counter.
func LoadBudget(sp *spool.Spool) Budget {
	b := Budget{Date: todayUTC(), Notices: map[string]string{}}
	if sp == nil {
		return b
	}
	raw, ok, err := sp.GetState(budgetKey)
	if err != nil || !ok || raw == "" {
		return b
	}
	var stored Budget
	if json.Unmarshal([]byte(raw), &stored) != nil {
		return b
	}
	if stored.Date != b.Date {
		return b
	}
	if stored.Notices == nil {
		stored.Notices = map[string]string{}
	}
	return stored
}

// SaveBudget persists the counter.
func SaveBudget(sp *spool.Spool, b Budget) {
	if sp == nil {
		return
	}
	if b.Notices == nil {
		b.Notices = map[string]string{}
	}
	raw, _ := json.Marshal(b)
	_ = sp.SetState(budgetKey, string(raw))
}

// Allow reports whether another model call is within maxCalls.
func (b *Budget) Allow(maxCalls int) bool {
	if maxCalls <= 0 {
		return true
	}
	if b.Date != todayUTC() {
		b.Date = todayUTC()
		b.Calls = 0
		b.InputTokens = 0
		b.OutputTokens = 0
	}
	return b.Calls < maxCalls
}

// Record adds usage from a provider result.
func (b *Budget) Record(res aiprovider.Result) {
	if b.Date != todayUTC() {
		b.Date = todayUTC()
		b.Calls = 0
		b.InputTokens = 0
		b.OutputTokens = 0
	}
	b.Calls++
	b.InputTokens += res.InputTokens
	b.OutputTokens += res.OutputTokens
}

// NoticeOnce is true if code has not been emitted today (and then records it).
func (b *Budget) NoticeOnce(code string) bool {
	if b.Notices == nil {
		b.Notices = map[string]string{}
	}
	day := todayUTC()
	if b.Notices[code] == day {
		return false
	}
	b.Notices[code] = day
	return true
}

// StatusItems reports spend on the board-client service.
func (b Budget) StatusItems() []event.StatusItem {
	return []event.StatusItem{
		{Key: "ai_calls_today", Label: "今日 AI 调用", Value: json.RawMessage(itoa(b.Calls)), ValueType: "number", Severity: "normal", DisplayFormat: "number", SortOrder: 80},
		{Key: "ai_input_tokens_today", Label: "今日 AI 输入 token", Value: json.RawMessage(itoa(b.InputTokens)), ValueType: "number", Severity: "normal", DisplayFormat: "number", SortOrder: 81},
	}
}

func itoa(n int) []byte {
	return []byte(strconv.Itoa(n))
}
