package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr              string
	Domain            string
	DatabasePath      string
	TeslaClientID     string
	TeslaClientSecret string
	RedirectURI       string
	FleetAPIBase      string
	AuthURL           string
	TokenURL          string
	PrivateKeyFile    string
	PublicKeyFile     string
	IngestToken       string
	AdminToken        string
	APNSKeyID         string
	APNSTeamID        string
	APNSKeyFile       string
	APNSBundleID      string
	APNSProduction    bool
	TelemetryHost     string
	TelemetryPort     int
	TelemetryCAFile   string
	DemoAutoplay      bool
	DocsDir           string
}

func LoadDotEnv(paths ...string) {
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, val, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			val = strings.Trim(strings.TrimSpace(val), `"'`)
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
		_ = f.Close()
	}
}

func FromEnv() Config {
	domain := env("DOMAIN", "skylar.snapcollectibles.com")
	cfg := Config{
		Addr:              env("ADDR", ":8080"),
		Domain:            domain,
		DatabasePath:      env("DATABASE_PATH", "data/honk.db"),
		TeslaClientID:     os.Getenv("TESLA_CLIENT_ID"),
		TeslaClientSecret: os.Getenv("TESLA_CLIENT_SECRET"),
		RedirectURI:       env("TESLA_REDIRECT_URI", "https://"+domain+"/path"),
		FleetAPIBase:      env("FLEET_API_BASE", "https://fleet-api.prd.na.vn.cloud.tesla.com"),
		AuthURL:           env("TESLA_AUTH_URL", "https://auth.tesla.com/oauth2/v3/authorize"),
		TokenURL:          env("TESLA_TOKEN_URL", "https://fleet-auth.prd.vn.cloud.tesla.com/oauth2/v3/token"),
		PrivateKeyFile:    os.Getenv("TESLA_PRIVATE_KEY_FILE"),
		PublicKeyFile:     os.Getenv("TESLA_PUBLIC_KEY_FILE"),
		IngestToken:       os.Getenv("TELEMETRY_INGEST_TOKEN"),
		AdminToken:        os.Getenv("ADMIN_TOKEN"),
		APNSKeyID:         os.Getenv("APNS_KEY_ID"),
		APNSTeamID:        os.Getenv("APNS_TEAM_ID"),
		APNSKeyFile:       os.Getenv("APNS_PRIVATE_KEY_FILE"),
		APNSBundleID:      env("APNS_BUNDLE_ID", "com.example.honkifyoureskylar"),
		APNSProduction:    strings.EqualFold(os.Getenv("APNS_ENV"), "production"),
		TelemetryHost:     env("TELEMETRY_HOST", domain),
		TelemetryPort:     envInt("TELEMETRY_PORT", 443),
		TelemetryCAFile:   os.Getenv("TELEMETRY_CA_FILE"),
		DemoAutoplay:      env("DEMO_AUTOPLAY", "true") != "false",
		DocsDir:           os.Getenv("DOCS_DIR"),
	}
	return cfg
}

func (c Config) TeslaConfigured() bool {
	return c.TeslaClientID != "" && c.TeslaClientSecret != ""
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
