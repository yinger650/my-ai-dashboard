package workspace

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"agentboard/internal/shared"
	"agentboard/internal/store"
)

const defaultMaxOpen = 32

// Tenant is a resolved workspace SQLite plus artifact directory.
type Tenant struct {
	Slug        string
	Store       *store.Store
	ArtifactDir string
	IngestBase  string
}

type cached struct {
	tenant *Tenant
	elem   *list.Element
}

// Hub opens per-slug board.db files with an LRU of live connections.
type Hub struct {
	dataDir string
	control *Control
	maxOpen int
	log     *slog.Logger

	mu    sync.Mutex
	lru   *list.List // front = most recently used; values are slug strings
	items map[string]*cached
}

func NewHub(dataDir string, control *Control, log *slog.Logger) *Hub {
	return &Hub{
		dataDir: dataDir,
		control: control,
		maxOpen: defaultMaxOpen,
		log:     log,
		lru:     list.New(),
		items:   map[string]*cached{},
	}
}

func (h *Hub) Control() *Control { return h.control }

func (h *Hub) Ping(ctx context.Context) error { return h.control.Ping(ctx) }

func (h *Hub) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for slug, it := range h.items {
		if it.tenant != nil && it.tenant.Store != nil {
			_ = it.tenant.Store.Close()
		}
		delete(h.items, slug)
	}
	h.lru.Init()
	return h.control.Close()
}

func (h *Hub) workspaceDir(slug string) string {
	return filepath.Join(h.dataDir, "workspaces", slug)
}

func (h *Hub) Open(ctx context.Context, slug string) (*Tenant, error) {
	if !ValidSlug(slug) {
		return nil, store.ErrNotFound
	}
	if _, err := h.control.GetWorkspaceBySlug(ctx, slug); err != nil {
		return nil, err
	}
	return h.openExisting(slug)
}

func (h *Hub) openExisting(slug string) (*Tenant, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if it, ok := h.items[slug]; ok {
		h.lru.MoveToFront(it.elem)
		return it.tenant, nil
	}
	dir := h.workspaceDir(slug)
	art := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(art, 0o750); err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(dir, "board.db"))
	if err != nil {
		return nil, err
	}
	if err := st.Migrate(); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("workspace %s migrate: %w", slug, err)
	}
	t := &Tenant{
		Slug:        slug,
		Store:       st,
		ArtifactDir: art,
		IngestBase:  "/" + slug,
	}
	for h.lru.Len() >= h.maxOpen {
		back := h.lru.Back()
		if back == nil {
			break
		}
		oldSlug := back.Value.(string)
		h.lru.Remove(back)
		if old, ok := h.items[oldSlug]; ok {
			if old.tenant != nil && old.tenant.Store != nil {
				_ = old.tenant.Store.Close()
			}
			delete(h.items, oldSlug)
		}
	}
	elem := h.lru.PushFront(slug)
	h.items[slug] = &cached{tenant: t, elem: elem}
	return t, nil
}

// EnsureUser creates or updates the Feishu user and their workspace.
func (h *Hub) EnsureUser(ctx context.Context, openID, unionID, tenantKey, displayName, avatarURL string) (*User, *Workspace, error) {
	u, err := h.control.GetUserByOpenID(ctx, openID)
	now := shared.FormatTime(shared.NowUTC())
	if err == nil {
		_ = h.control.UpdateUserProfile(ctx, u.ID, displayName, avatarURL, tenantKey)
		u.DisplayName = displayName
		u.AvatarURL = avatarURL
		u.TenantKey = tenantKey
		ws, werr := h.control.GetWorkspaceByOwner(ctx, u.ID)
		if werr != nil {
			return nil, nil, werr
		}
		if _, oerr := h.openExisting(ws.Slug); oerr != nil {
			return nil, nil, oerr
		}
		return u, ws, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return nil, nil, err
	}

	u = &User{
		ID:            newID(),
		FeishuOpenID:  openID,
		FeishuUnionID: unionID,
		TenantKey:     tenantKey,
		DisplayName:   displayName,
		AvatarURL:     avatarURL,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := h.control.InsertUser(ctx, u); err != nil {
		return nil, nil, err
	}
	var slug string
	for i := 0; i < 16; i++ {
		s, rerr := randomSlug()
		if rerr != nil {
			return nil, nil, rerr
		}
		taken, terr := h.control.SlugTaken(ctx, s)
		if terr != nil {
			return nil, nil, terr
		}
		if !taken {
			slug = s
			break
		}
	}
	if slug == "" {
		return nil, nil, fmt.Errorf("could not allocate workspace slug")
	}
	ws := &Workspace{ID: newID(), Slug: slug, OwnerUserID: u.ID, CreatedAt: now}
	if err := os.MkdirAll(filepath.Join(h.workspaceDir(slug), "artifacts"), 0o750); err != nil {
		return nil, nil, err
	}
	if err := h.control.InsertWorkspace(ctx, ws); err != nil {
		return nil, nil, err
	}
	if _, err := h.openExisting(slug); err != nil {
		return nil, nil, err
	}
	return u, ws, nil
}

func (h *Hub) ForEachStore(ctx context.Context, fn func(slug string, st *store.Store) error) error {
	slugs, err := h.control.ListSlugs(ctx)
	if err != nil {
		return err
	}
	for _, slug := range slugs {
		t, err := h.Open(ctx, slug)
		if err != nil {
			if h.log != nil {
				h.log.Warn("open workspace for maintenance failed", "slug", slug, "err", err)
			}
			continue
		}
		if err := fn(slug, t.Store); err != nil {
			return err
		}
	}
	return nil
}
