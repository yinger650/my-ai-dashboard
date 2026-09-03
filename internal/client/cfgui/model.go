package cfgui

import (
	"os"
	"path/filepath"
	"strings"

	"agentboard/internal/client/config"
	"agentboard/internal/client/spool"
)

// Model is the shared TUI/WEB editor state.
type Model struct {
	Path         string
	NewFile      bool
	URL          string
	Key          string
	Name         string
	Token        string
	Enabled      map[string]bool
	Subs         map[string]map[string]bool
	Unseen       map[string]bool
	Probes       []config.StatusProbe
	HTTP         []config.HTTPTarget
	Scripts      []config.ProbeScript
	TouchP       bool
	TouchH       bool
	TouchS       bool
	origEn       map[string]bool
	origSub      map[string]map[string]bool
	origURL      string
	origKey      string
	origName     string
	tokenTouched bool
}

func loadModel(path string) (*Model, error) {
	doc, cfg, missing, err := config.LoadDocument(path)
	if err != nil {
		return nil, err
	}
	m := &Model{
		Path:    path,
		NewFile: missing,
		Enabled: map[string]bool{},
		Subs:    map[string]map[string]bool{},
		Unseen:  map[string]bool{},
		origEn:  map[string]bool{},
		origSub: map[string]map[string]bool{},
	}
	if missing {
		m.URL = "https://board.yinger650.com"
		m.Key = "home-server"
		m.origURL, m.origKey = "", ""
		for _, f := range config.Catalog() {
			m.Enabled[f.ID] = f.DefaultOn
			m.origEn[f.ID] = false
			if len(f.Subs) > 0 {
				m.Subs[f.ID] = map[string]bool{}
				m.origSub[f.ID] = map[string]bool{}
				for _, s := range f.Subs {
					m.Subs[f.ID][s.ID] = false
					m.origSub[f.ID][s.ID] = false
				}
			}
		}
		return m, nil
	}
	m.URL = cfg.Server.URL
	m.Key = cfg.Machine.Key
	m.Name = cfg.Machine.DisplayName
	m.Token = cfg.Server.MachineToken
	m.origURL, m.origKey, m.origName = m.URL, m.Key, m.Name
	m.Probes = append([]config.StatusProbe(nil), cfg.Machine.StatusProbes...)
	m.HTTP = append([]config.HTTPTarget(nil), cfg.Collectors.HTTP.Targets...)
	m.Scripts = append([]config.ProbeScript(nil), cfg.Collectors.Probes.Scripts...)
	root := config.DocRoot(doc)
	for _, f := range config.Catalog() {
		on := config.FeatureEnabled(root, f)
		m.Enabled[f.ID] = on
		m.origEn[f.ID] = on
		if len(f.Subs) > 0 {
			m.Subs[f.ID] = map[string]bool{}
			m.origSub[f.ID] = map[string]bool{}
			for _, s := range f.Subs {
				subOn := config.SubEnabled(root, f.ID, s.ID)
				m.Subs[f.ID][s.ID] = subOn
				m.origSub[f.ID][s.ID] = subOn
			}
		}
	}
	seen := config.ParseSeenIDs(readSeen(config.SpoolPathFromDoc(root)))
	effective := config.EffectiveSeen(seen, cfg)
	for _, id := range config.UnseenIDs(effective) {
		m.Unseen[id] = true
	}
	return m, nil
}

func (m *Model) edit() config.Edit {
	feats := map[string]bool{}
	subs := map[string]map[string]bool{}
	for id, want := range m.Enabled {
		if m.origEn[id] != want {
			feats[id] = want
		}
	}
	for parent, sm := range m.Subs {
		changed := m.origEn[parent] != m.Enabled[parent]
		if !changed {
			for sid, want := range sm {
				if m.origSub[parent][sid] != want {
					changed = true
					break
				}
			}
		}
		if !changed {
			continue
		}
		feats[parent] = m.Enabled[parent]
		cp := map[string]bool{}
		for k, v := range sm {
			cp[k] = v
		}
		subs[parent] = cp
	}
	ed := config.Edit{
		StatusProbes: m.Probes,
		HTTPTargets:  m.HTTP,
		ProbeScripts: m.Scripts,
		WriteProbes:  m.TouchP,
		WriteHTTP:    m.TouchH,
		WriteScripts: m.TouchS,
		Features:     feats,
		Subs:         subs,
	}
	if strings.TrimSpace(m.URL) != m.origURL {
		ed.URL = strings.TrimSpace(m.URL)
	}
	if strings.TrimSpace(m.Key) != m.origKey {
		ed.MachineKey = strings.TrimSpace(m.Key)
	}
	if strings.TrimSpace(m.Name) != m.origName {
		ed.DisplayName = strings.TrimSpace(m.Name)
	}
	if m.tokenTouched {
		ed.Token = strings.TrimSpace(m.Token)
	}
	return ed
}

func readSeen(spoolPath string) string {
	if spoolPath == "" {
		return ""
	}
	sp, err := spool.Open(spoolPath)
	if err != nil {
		return ""
	}
	defer sp.Close()
	v, _, _ := sp.GetState(config.SeenFeaturesKey)
	return v
}

func markCatalogSeen(spoolPath string) {
	if spoolPath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(spoolPath), 0o750); err != nil {
		return
	}
	sp, err := spool.Open(spoolPath)
	if err != nil {
		return
	}
	defer sp.Close()
	_ = sp.SetState(config.SeenFeaturesKey, config.EncodeSeenIDs(config.AllCatalogIDs()))
}

func maskToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "abp_m_REPLACE_ME" {
		return "(空)"
	}
	if len(s) <= 10 {
		return "****"
	}
	return s[:8] + "…"
}

func isNew(m *Model, id string) bool {
	return m.Unseen[id]
}
