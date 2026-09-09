package feishu

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// User is the subset of Feishu user_info used for workspace ownership.
type User struct {
	OpenID      string
	UnionID     string
	TenantKey   string
	DisplayName string
	AvatarURL   string
}

// OAuth talks to Feishu Open Platform.
type OAuth struct {
	AppID        string
	AppSecret    string
	RedirectURI  string
	APIBase      string
	AuthorizeURL string
	HTTPClient   *http.Client
}

func (o *OAuth) client() *http.Client {
	if o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// AuthCodeURL is the browser redirect to Feishu.
func (o *OAuth) AuthCodeURL(state string) string {
	q := url.Values{}
	q.Set("client_id", o.AppID)
	q.Set("redirect_uri", o.RedirectURI)
	q.Set("response_type", "code")
	q.Set("state", state)
	return strings.TrimRight(o.AuthorizeURL, "?") + "?" + q.Encode()
}

func (o *OAuth) Exchange(code string) (accessToken string, err error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("client_id", o.AppID)
	body.Set("client_secret", o.AppSecret)
	body.Set("code", code)
	body.Set("redirect_uri", o.RedirectURI)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(o.APIBase, "/")+"/open-apis/authen/v2/oauth/token", strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := o.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var out struct {
		Code        int    `json:"code"`
		Msg         string `json:"msg"`
		AccessToken string `json:"access_token"`
		Data        struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("feishu token json: %w", err)
	}
	if out.Code != 0 {
		return "", fmt.Errorf("feishu token: code=%d msg=%s", out.Code, out.Msg)
	}
	tok := out.AccessToken
	if tok == "" {
		tok = out.Data.AccessToken
	}
	if tok == "" {
		return "", fmt.Errorf("feishu token: empty access_token")
	}
	return tok, nil
}

func (o *OAuth) UserInfo(accessToken string) (*User, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(o.APIBase, "/")+"/open-apis/authen/v1/user_info", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := o.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID    string `json:"open_id"`
			UnionID   string `json:"union_id"`
			TenantKey string `json:"tenant_key"`
			Name      string `json:"name"`
			AvatarURL string `json:"avatar_url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("feishu user_info json: %w", err)
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("feishu user_info: code=%d msg=%s", out.Code, out.Msg)
	}
	if out.Data.OpenID == "" {
		return nil, fmt.Errorf("feishu user_info: empty open_id")
	}
	return &User{
		OpenID:      out.Data.OpenID,
		UnionID:     out.Data.UnionID,
		TenantKey:   out.Data.TenantKey,
		DisplayName: out.Data.Name,
		AvatarURL:   out.Data.AvatarURL,
	}, nil
}
