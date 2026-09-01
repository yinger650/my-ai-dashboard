package aiinspect

import (
	"encoding/json"

	"agentboard/internal/client/spool"
)

const (
	bufKey      = "ai.logbuf"
	defaultMaxN = 200
	defaultMaxB = 256 * 1024
)

// Entry is one teed log.append.
type Entry struct {
	ServiceKey string `json:"service_key"`
	Markdown   string `json:"markdown"`
	Severity   string `json:"severity"`
	OccurredAt string `json:"occurred_at"`
	Source     string `json:"source"`
}

type bufState struct {
	Items []Entry `json:"items"`
}

// Buffer is a bounded ring of recent logs persisted in client_state.
type Buffer struct {
	sp       *spool.Spool
	maxN     int
	maxBytes int
}

// NewBuffer stores recent logs in spool client_state.
func NewBuffer(sp *spool.Spool) *Buffer {
	return &Buffer{sp: sp, maxN: defaultMaxN, maxBytes: defaultMaxB}
}

// Append adds an entry and trims the ring.
func (b *Buffer) Append(e Entry) error {
	if b == nil || b.sp == nil {
		return nil
	}
	st, err := b.load()
	if err != nil {
		return err
	}
	st.Items = append(st.Items, e)
	st.Items = trimBuf(st.Items, b.maxN, b.maxBytes)
	return b.save(st)
}

// Recent returns the newest matching entries (oldest-first for summarization).
func (b *Buffer) Recent(source string, n int) []Entry {
	if b == nil || b.sp == nil {
		return nil
	}
	st, err := b.load()
	if err != nil {
		return nil
	}
	var matched []Entry
	for _, it := range st.Items {
		if source == "" || source == "agent_logs" || it.Source == source {
			matched = append(matched, it)
		}
	}
	if n > 0 && len(matched) > n {
		matched = matched[len(matched)-n:]
	}
	return matched
}

func (b *Buffer) load() (bufState, error) {
	raw, ok, err := b.sp.GetState(bufKey)
	if err != nil || !ok || raw == "" {
		return bufState{}, err
	}
	var st bufState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return bufState{}, nil
	}
	return st, nil
}

func (b *Buffer) save(st bufState) error {
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return b.sp.SetState(bufKey, string(raw))
}

func trimBuf(items []Entry, maxN, maxBytes int) []Entry {
	if maxN <= 0 {
		maxN = defaultMaxN
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxB
	}
	for len(items) > maxN {
		items = items[1:]
	}
	total := 0
	for _, it := range items {
		total += len(it.Markdown)
	}
	for total > maxBytes && len(items) > 1 {
		total -= len(items[0].Markdown)
		items = items[1:]
	}
	return items
}
