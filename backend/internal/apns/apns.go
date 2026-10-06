package apns

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Client struct {
	key    *ecdsa.PrivateKey
	keyID  string
	teamID string
	bundle string
	host   string
	http   *http.Client

	mu    sync.Mutex
	token string
	until time.Time
}

func New(keyFile, keyID, teamID, bundle string, production bool) (*Client, error) {
	if keyFile == "" || keyID == "" || teamID == "" {
		return nil, fmt.Errorf("apns is not configured")
	}
	raw, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("apns key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("apns key is not ECDSA")
	}
	host := "https://api.sandbox.push.apple.com"
	if production {
		host = "https://api.push.apple.com"
	}
	return &Client{
		key: key, keyID: keyID, teamID: teamID, bundle: bundle, host: host,
		http: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *Client) Notify(deviceToken, title, body string) error {
	if c == nil || deviceToken == "" {
		return nil
	}
	jwtToken, err := c.bearer()
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{"title": title, "body": body},
			"sound": "default",
		},
	})
	req, err := http.NewRequest(http.MethodPost, c.host+"/3/device/"+deviceToken, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "bearer "+jwtToken)
	req.Header.Set("apns-topic", c.bundle)
	req.Header.Set("apns-push-type", "alert")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("apns returned %d", res.StatusCode)
	}
	return nil
}

func (c *Client) bearer() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.until) {
		return c.token, nil
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": c.teamID,
		"iat": time.Now().Unix(),
	})
	tok.Header["kid"] = c.keyID
	signed, err := tok.SignedString(c.key)
	if err != nil {
		return "", err
	}
	c.token = signed
	c.until = time.Now().Add(50 * time.Minute)
	return signed, nil
}
