package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/model"
	"github.com/christricia2009-star/honkifyoureskylar/backend/internal/telemetry"
)

type Vehicle struct {
	VIN    string
	Name   string
	State  string
	Access string
}

type StatusError struct {
	Status int
	Body   string
	Path   string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("fleet %s returned %d", e.Path, e.Status)
}

type TokenFunc func(ctx context.Context) (string, error)

type Client struct {
	Base  string
	Token TokenFunc
	HTTP  *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) List(ctx context.Context) ([]Vehicle, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/1/vehicles", nil, &raw); err != nil {
		return nil, err
	}
	return vehiclesFrom(raw), nil
}

func (c *Client) Get(ctx context.Context, vin string) (Vehicle, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/1/vehicles/"+url.PathEscape(vin), nil, &raw); err != nil {
		return Vehicle{}, err
	}
	list := vehiclesFrom(raw)
	if len(list) == 0 {
		if v, ok := oneVehicle(raw); ok {
			return v, nil
		}
		return Vehicle{VIN: vin, State: "offline"}, nil
	}
	return list[0], nil
}

func (c *Client) Wake(ctx context.Context, vin string) (Vehicle, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodPost, "/api/1/vehicles/"+url.PathEscape(vin)+"/wake_up", map[string]any{}, &raw); err != nil {
		return Vehicle{}, err
	}
	if v, ok := oneVehicle(raw); ok {
		return v, nil
	}
	return Vehicle{VIN: vin, State: "asleep"}, nil
}

func (c *Client) Data(ctx context.Context, vin string) (telemetry.Update, Vehicle, error) {
	var raw map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/1/vehicles/"+url.PathEscape(vin)+"/vehicle_data", nil, &raw); err != nil {
		return telemetry.Update{}, Vehicle{}, err
	}
	up, vehicle := ParseVehicleData(vin, raw)
	return up, vehicle, nil
}

func (c *Client) Drivers(ctx context.Context, vin string) (int, []model.Driver, string, error) {
	token, err := c.Token(ctx)
	if err != nil {
		return 0, nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/api/1/vehicles/"+url.PathEscape(vin)+"/drivers", nil)
	if err != nil {
		return 0, nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := c.http().Do(req)
	if err != nil {
		return 0, nil, "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusUnauthorized {
		return res.StatusCode, nil, "Tesla refused the driver list for this car. Sign-in already includes vehicle_device_data. Tesla only gives that list to the owner of this specific car.", nil
	}
	if res.StatusCode >= 300 {
		return res.StatusCode, nil, fmt.Sprintf("Tesla returned %d for the driver list.", res.StatusCode), nil
	}
	return res.StatusCode, ParseDrivers(body), "", nil
}

// Do performs one Fleet API call and returns the status and body, including Tesla's error body.
func (c *Client) Do(ctx context.Context, method, path string, in any) (int, []byte, error) {
	token, err := c.Token(ctx)
	if err != nil {
		return 0, nil, err
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http().Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	return res.StatusCode, buf, nil
}

func (c *Client) do(ctx context.Context, method, path string, in any, out any) error {
	status, buf, err := c.Do(ctx, method, path, in)
	if err != nil {
		return err
	}
	if status >= 300 {
		return &StatusError{Status: status, Body: trim(buf), Path: path}
	}
	if out == nil || len(buf) == 0 {
		return nil
	}
	return json.Unmarshal(buf, out)
}

func (c *Client) base() string {
	return strings.TrimRight(c.Base, "/")
}

func trim(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
