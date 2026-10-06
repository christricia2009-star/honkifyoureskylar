package command

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/golang-jwt/jwt/v5"
	"github.com/teslamotors/vehicle-command/pkg/protocol"
	"github.com/teslamotors/vehicle-command/pkg/proxy"
	"github.com/teslamotors/vehicle-command/pkg/sign"
)

// Signer holds the Fleet API virtual-key private key. It never leaves the server.
type Signer struct {
	key   protocol.ECDHPrivateKey
	proxy *proxy.Proxy
}

func Load(path string) (*Signer, error) {
	key, err := protocol.LoadPrivateKey(path)
	if err != nil {
		return nil, err
	}
	p, err := proxy.New(context.Background(), key, 8)
	if err != nil {
		return nil, err
	}
	return &Signer{key: key, proxy: p}, nil
}

// SignFleetConfig signs a Fleet Telemetry config with Tesla's Schnorr scheme.
// The audience is com.tesla.fleet.TelemetryClient, which is what the cars expect.
func (s *Signer) SignFleetConfig(config map[string]any) (string, error) {
	claims := jwt.MapClaims{}
	for k, v := range config {
		claims[k] = v
	}
	return sign.SignMessageForFleet(s.key, "TelemetryClient", claims)
}

// PostTelemetryConfig sends the unsigned config through the vehicle-command
// proxy, which signs it and posts the JWS to Fleet API.
// PostCommand signs one vehicle command and posts it through the vehicle-command proxy.
func (s *Signer) PostCommand(ctx context.Context, teslaToken, vin, name string, payload []byte) (int, []byte, error) {
	req := httptest.NewRequest(http.MethodPost, "https://fleet.local/api/1/vehicles/"+vin+"/command/"+name, bytes.NewReader(payload))
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+teslaToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.proxy.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes(), nil
}

func (s *Signer) PostTelemetryConfig(ctx context.Context, teslaToken string, payload []byte) (int, []byte, error) {
	req := httptest.NewRequest(http.MethodPost, "https://fleet.local/api/1/vehicles/fleet_telemetry_config", bytes.NewReader(payload))
	req = req.WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+teslaToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.proxy.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes(), nil
}
