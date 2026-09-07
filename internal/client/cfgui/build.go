package cfgui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/config"
	"agentboard/internal/client/statusprobe"
)

func previewText(p statusprobe.Preview) string {
	var b strings.Builder
	if p.Error != "" {
		b.WriteString("错误: ")
		b.WriteString(p.Error)
		if p.Output != "" {
			b.WriteString("\n")
		}
	}
	b.WriteString(p.Output)
	return strings.TrimSpace(b.String())
}

func (m *Model) attachExt(cfg *config.Config, spoolPath string) {
	if spoolPath == "" {
		spoolPath = "/var/lib/agentboard-client/spool.db"
	}
	m.ExtRoot = filepath.Join(filepath.Dir(spoolPath), "extensions")
	m.LegacyDir = filepath.Join(filepath.Dir(spoolPath), "probes")
	if cfg != nil {
		if strings.TrimSpace(cfg.Storage.ExtensionsPath) != "" {
			m.ExtRoot = cfg.Storage.ExtensionsPath
		} else if cfg.Storage.SpoolPath != "" {
			m.ExtRoot = cfg.ExtensionsRoot()
			m.LegacyDir = cfg.ProbeDir()
		}
		m.AIEnabled = cfg.AI.Enabled
	}
	if m.Previews == nil {
		m.Previews = map[string]string{}
	}
	for _, p := range m.Probes {
		dir := strings.TrimSpace(p.Dir)
		if dir == "" {
			dir = config.NLRelDir(p.Key)
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(m.ExtRoot, dir)
		}
		prev := statusprobe.ReadPreview(dir)
		if text := previewText(prev); text != "" {
			m.Previews[p.Key] = text
		}
	}
}

func (m *Model) probeBuilt(p config.StatusProbe) bool {
	return statusprobe.Built(p, m.ExtRoot, m.LegacyDir)
}

func (m *Model) newCompiler() (*statusprobe.Compiler, error) {
	ext, legacy := m.ExtRoot, m.LegacyDir
	aiOn := m.AIEnabled
	var cfg *config.Config
	if m.Path != "" {
		if c, err := config.Read(m.Path); err == nil {
			cfg = c
			ext = c.ExtensionsRoot()
			legacy = c.ProbeDir()
			aiOn = c.AI.Enabled
		}
	}
	if ext == "" {
		ext = filepath.Join(os.TempDir(), "agentboard-extensions")
	}
	prov := m.Provider
	if prov == nil && cfg != nil && cfg.AI.Enabled {
		p, err := aiprovider.New(aiprovider.Options{
			Provider:  cfg.AI.Provider,
			Command:   cfg.AI.Command,
			Model:     cfg.AI.Model,
			APIKeyEnv: cfg.AI.APIKeyEnv,
			Workspace: cfg.AI.Workspace,
		})
		if err == nil {
			prov = p
		}
	}
	if m.Provider != nil {
		aiOn = true
	}
	return &statusprobe.Compiler{
		Dir: ext, LegacyDir: legacy, Provider: prov, AIEnabled: aiOn,
	}, nil
}

func (m *Model) buildAt(idx int) (statusprobe.Preview, error) {
	if idx < 0 || idx >= len(m.Probes) {
		return statusprobe.Preview{}, fmt.Errorf("编号无效")
	}
	p := m.Probes[idx]
	if strings.TrimSpace(p.Key) == "" {
		return statusprobe.Preview{}, fmt.Errorf("key 为空")
	}
	if strings.TrimSpace(p.Intent) == "" && len(p.Command) == 0 {
		return statusprobe.Preview{}, fmt.Errorf("请先填写自然语言描述")
	}
	comp, err := m.newCompiler()
	if err != nil {
		return statusprobe.Preview{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ready, prev, ok := comp.PrepareOne(ctx, p)
	if m.Previews == nil {
		m.Previews = map[string]string{}
	}
	m.Previews[p.Key] = previewText(prev)
	m.TouchP = true
	if ok {
		statusprobe.ApplyBuildResult(&m.Probes[idx], ready)
	}
	if !prev.OK && prev.Error != "" && !ok {
		return prev, fmt.Errorf("%s", prev.Error)
	}
	return prev, nil
}

func (m *Model) supplementAt(idx int, extra string) error {
	if idx < 0 || idx >= len(m.Probes) {
		return fmt.Errorf("编号无效")
	}
	m.Probes[idx].Supplement(extra)
	m.TouchP = true
	return nil
}

func (m *Model) toggleEnableAt(idx int) error {
	if idx < 0 || idx >= len(m.Probes) {
		return fmt.Errorf("编号无效")
	}
	p := m.Probes[idx]
	if !m.probeBuilt(p) {
		return fmt.Errorf("请先 Build 成功再启用")
	}
	on := !p.IsEnabled()
	m.Probes[idx].Enabled = config.BoolPtr(on)
	m.TouchP = true
	return nil
}
