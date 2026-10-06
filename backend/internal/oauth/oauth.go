package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/config"
)

const Scopes = "openid offline_access user_data vehicle_device_data vehicle_location vehicle_cmds"

type Token struct {
	Access  string
	Refresh string
	Expiry  time.Time
	Scope   string
}

func AuthorizeURL(cfg config.Config, state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", cfg.TeslaClientID)
	q.Set("redirect_uri", cfg.RedirectURI)
	q.Set("scope", Scopes)
	q.Set("state", state)
	q.Set("prompt", "login")
	q.Set("prompt_missing_scopes", "true")
	q.Set("require_requested_scopes", "true")
	q.Set("show_keypair_step", "true")
	return cfg.AuthURL + "?" + q.Encode()
}

func Exchange(ctx context.Context, cfg config.Config, code string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", cfg.TeslaClientID)
	form.Set("client_secret", cfg.TeslaClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", cfg.RedirectURI)
	form.Set("audience", cfg.FleetAPIBase)
	return postToken(ctx, cfg.TokenURL, form)
}

func Refresh(ctx context.Context, cfg config.Config, refreshToken string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", cfg.TeslaClientID)
	form.Set("refresh_token", refreshToken)
	return postToken(ctx, cfg.TokenURL, form)
}

func PartnerToken(ctx context.Context, cfg config.Config) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", cfg.TeslaClientID)
	form.Set("client_secret", cfg.TeslaClientSecret)
	form.Set("audience", cfg.FleetAPIBase)
	form.Set("scope", "openid")
	tok, err := postToken(ctx, cfg.TokenURL, form)
	if err != nil {
		return "", err
	}
	return tok.Access, nil
}

func Register(ctx context.Context, cfg config.Config, partnerToken string) (int, string, error) {
	body := `{"domain":"` + cfg.Domain + `"}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.FleetAPIBase, "/")+"/api/1/partner_accounts", strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+partnerToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return res.StatusCode, string(buf), nil
}

func postToken(ctx context.Context, endpoint string, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Token{}, err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return Token{}, fmt.Errorf("tesla token endpoint returned %d", res.StatusCode)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(buf, &raw); err != nil {
		return Token{}, err
	}
	if raw.AccessToken == "" {
		return Token{}, fmt.Errorf("tesla token response had no access token")
	}
	exp := time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second)
	if raw.ExpiresIn == 0 {
		exp = time.Now().Add(8 * time.Hour)
	}
	return Token{Access: raw.AccessToken, Refresh: raw.RefreshToken, Expiry: exp, Scope: raw.Scope}, nil
}
