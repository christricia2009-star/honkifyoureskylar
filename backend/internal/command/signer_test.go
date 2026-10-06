package command

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignFleetConfigUsesTeslaSchnorr(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "private-key.pem")
	raw := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	signer, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.SignFleetConfig(map[string]any{
		"hostname": "skylar.snapcollectibles.com",
		"port":     443,
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt parts %d", len(parts))
	}
	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var hdr map[string]any
	if err := json.Unmarshal(header, &hdr); err != nil {
		t.Fatal(err)
	}
	if hdr["alg"] != "Tesla.SS256" {
		t.Fatalf("alg %#v", hdr["alg"])
	}
}
