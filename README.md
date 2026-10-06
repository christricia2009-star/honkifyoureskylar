# Honk if You're Skylar

A private iOS app and a small Go server for one owner and one car. It uses the official Tesla Fleet API. It does not scrape the Tesla app, it does not store the Tesla password, and it does not put the Fleet API client secret or the virtual-key private key in the iOS binary.

The phone signs in through the owner's server. Tesla tokens stay on the server. The app keeps only a short-lived handoff, then a session token in the keychain.

## What is already true about the domain

`skylar.snapcollectibles.com` is served by the Vercel project `honkifyoureskylar` (team `true-family`). DNS is a CNAME to Vercel, and the certificate is Let's Encrypt. These URLs return HTTP 200:

- `https://skylar.snapcollectibles.com/`
- `https://skylar.snapcollectibles.com/path`
- `https://skylar.snapcollectibles.com/privacy`
- `https://skylar.snapcollectibles.com/.well-known/appspecific/com.tesla.3p.public-key.pem`

GitHub Pages is not the host. Do not point this name at `github.io`. That would take the domain away from the site Tesla can already fetch.

The Vercel project's root directory is `docs`. A production deploy of the repository root serves an empty site and those URLs return 404. Leave the root directory on `docs`.

The static site is only the public proof of those URLs. Sign in with Tesla finishes only when the Go server receives `GET /path?code=...`, exchanges the code, and redirects to `honkifyoureskylar://auth?code=...`. Until the Go server owns the domain, use the local demo.

## Tesla developer portal

The application **Honk if You're Skylar** was approved, and the North America partner account was registered on 2026-10-06. Tesla stored the hosted public key for `skylar.snapcollectibles.com`. Leave the client URLs exactly as they are:

| Field | Value |
| --- | --- |
| Allowed Origin | `https://skylar.snapcollectibles.com` |
| Allowed Redirect URI | `https://skylar.snapcollectibles.com/path` |
| Allowed Returned URL | `https://skylar.snapcollectibles.com` |

The earlier rejection, "Invalid Origin, Redirect, or Return URI/URL", was a 404 from an empty Vercel project. The certificate was already valid. Do not change the three URLs.

OAuth grant: Authorization Code and Machine-to-Machine (authorization code and client credentials).

Scopes, in one string:

```text
openid offline_access user_data vehicle_device_data vehicle_location vehicle_cmds
```

`vehicle_location` makes the car show the location-sharing icon. That is expected.

The server also accepts `/auth/tesla/callback` as an alias. The portal value that must match is `/path`, because that is what the form has. `TESLA_REDIRECT_URI` must be the same string you submit. The default is `https://skylar.snapcollectibles.com/path`.

Authorize URL extras the server sends: `prompt=login`, `prompt_missing_scopes=true`, `require_requested_scopes=true`, `show_keypair_step=true`.

## Keys

A P-256 key pair already exists on the machine that created this repo. The private key is `backend/secrets/private-key.pem` (mode 600, gitignored). The public key is committed and hosted. Back up the private key. If it is lost, generate a new pair, replace the hosted PEM, register again, and pair the car again.

To create a pair yourself:

```bash
mkdir -p backend/secrets
openssl ecparam -name prime256v1 -genkey -noout -out backend/secrets/private-key.pem
chmod 600 backend/secrets/private-key.pem
openssl ec -in backend/secrets/private-key.pem -pubout \
  -out docs/.well-known/appspecific/com.tesla.3p.public-key.pem
```

The public key must stay at:

```text
https://skylar.snapcollectibles.com/.well-known/appspecific/com.tesla.3p.public-key.pem
```

Content type `application/x-pem-file`. When you later point the domain at the Go server, that process has to keep serving the same file. Pairing and registration both depend on it.

Virtual-key pairing, done in the Tesla app before live data and commands work:

```text
https://www.tesla.com/_ak/skylar.snapcollectibles.com
```

The Setup sheet in the app explains that step. Honk cannot see the car's key list, so the owner marks "virtual key paired" by hand. A successful telemetry configuration also marks it.

## Credentials

Copy `backend/.env.example` to `backend/.env`. Git ignores `.env`. Put `TESLA_CLIENT_ID` and `TESLA_CLIENT_SECRET` only there. The iOS app never sees them. The server logs whether Tesla is configured. It does not log the secret or the tokens.

Empty client id and secret leave the server in demo mode. A demo session still exists after you fill them in.

Run the server from `backend/` so the relative paths in `.env` resolve:

```bash
export PATH="$HOME/.local/go/bin:$PATH"
cd backend
go run ./cmd/honk-server
```

On this Mac, Go is `go1.25.4` at `$HOME/.local/go`, not on the default `PATH`. The module asks for Go 1.26 because of the SQLite driver. That install downloads the newer toolchain on the first `go test` or `go run`.

Local API base: `http://127.0.0.1:8080`. The iOS app allows local networking. Demo:

```bash
curl -s -X POST http://127.0.0.1:8080/api/session/demo
curl -s http://127.0.0.1:8080/api/health
```

## Register the partner account

This was done for North America on 2026-10-06. Tesla returned the domain and the same public key that is hosted above. Register again only for another region, or after replacing the key pair. The domain must match the root of the allowed origin. The commands below are the ones that were used:

From `backend/`, with `.env` loaded:

```bash
set -a && source .env && set +a
curl -s -X POST "$TESLA_TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=client_credentials" \
  -d "client_id=$TESLA_CLIENT_ID" \
  -d "client_secret=$TESLA_CLIENT_SECRET" \
  -d "audience=$FLEET_API_BASE" \
  -d "scope=openid"
```

Then, with the access token from that response:

```bash
curl -s -X POST "$FLEET_API_BASE/api/1/partner_accounts" \
  -H "Authorization: Bearer $PARTNER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"domain":"skylar.snapcollectibles.com"}'
```

Or start the server and call the admin route. It answers 404 unless `ADMIN_TOKEN` is set:

```bash
curl -s -X POST http://127.0.0.1:8080/admin/register \
  -H "Authorization: Bearer $ADMIN_TOKEN"
```

Fleet API base for North America, the default:

```text
https://fleet-api.prd.na.vn.cloud.tesla.com
```

Auth: `https://auth.tesla.com/oauth2/v3/authorize`

Token: `https://fleet-auth.prd.vn.cloud.tesla.com/oauth2/v3/token`

User tokens use `authorization_code` with the client secret on the server and `audience` set to the fleet base. Refresh uses `grant_type`, `client_id`, and `refresh_token`. If Tesla returns a new refresh token, the server stores it.

## Environment

| Variable | Purpose |
| --- | --- |
| `TESLA_CLIENT_ID` | Fleet API app id. Server only. |
| `TESLA_CLIENT_SECRET` | Fleet API secret. Server only. |
| `DOMAIN` | `skylar.snapcollectibles.com` |
| `TESLA_REDIRECT_URI` | Must equal the portal redirect. Default `https://DOMAIN/path`. |
| `FLEET_API_BASE` | Regional Fleet API host. |
| `TESLA_AUTH_URL` | Authorize endpoint. |
| `TESLA_TOKEN_URL` | Token endpoint. |
| `TESLA_PRIVATE_KEY_FILE` | P-256 private key. Gitignored. |
| `TESLA_PUBLIC_KEY_FILE` | PEM the server and the static site share. |
| `DOCS_DIR` | Static pages for `/`, `/path`, and `/privacy`. |
| `DATABASE_PATH` | SQLite file. Default `data/honk.db`. |
| `ADDR` | Default `:8080`. |
| `TELEMETRY_INGEST_TOKEN` | Bearer for `POST /ingest/telemetry`. Empty means loopback only. |
| `ADMIN_TOKEN` | Bearer for `POST /admin/register`. Empty means 404. |
| `TELEMETRY_HOST` | Hostname of the Fleet Telemetry server. Not this static site. |
| `TELEMETRY_PORT` | Default `443`. |
| `TELEMETRY_CA_FILE` | CA chain the car should trust for that telemetry server. |
| `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_PRIVATE_KEY_FILE` | Optional `.p8` token auth. |
| `APNS_BUNDLE_ID` | Default `com.example.honkifyoureskylar`. |
| `APNS_ENV` | `production` for the production APNs host. Otherwise sandbox. |
| `DEMO_AUTOPLAY` | `false` freezes the scripted demo drive. |

Quoted values are fine. The loader strips one pair of surrounding quotes and does not override variables that are already set.

## Billing

Tesla's published case studies imply these estimates. They are not a live rate card. Confirm the table in the portal. The partner account is on the pay-as-you-go tier. Add a card in the developer portal and set the billing cap to $10. A cap of $0 disables the API.

| Call | Estimate |
| --- | --- |
| Streaming signal | $0.00000667 |
| `vehicle_data` | $0.002 |
| Signed command | $0.001 |
| Wake | $0.02 |

Vehicle list and the driver list are counted as metadata and priced at $0 in the estimate, because the case studies did not price them. Auth is not billed. A response status below 500 is billable. A network error is not.

The working assumption is Tesla's $10 monthly credit. Set a portal billing cap of $10 so the credit covers usage. A missing payment method, or a cap of $0, gets the app disabled. Honk does not poll `vehicle_data` on a timer. Pull to refresh is one confirmed peek. If the car is asleep, that peek also wakes it, and the app says so before it spends the call. Offline cars are left alone. The garage shows the last time the car was heard from.

## Data the app will and will not claim

`GET /api/1/vehicles/{vin}/drivers` is the owner allow-list, not the in-car profile. Honk shows the seat, guest mode, and that list. It does not invent the occupant. If the scope is missing, the driver screen says so and the list is empty.

Guest mode with an occupied seat is not Skylar. The horn may still talk about Skylar on the owner's own trips. The fields stay literal.

`vehicle_data` treats a null shift state as Park and a null speed as 0. It does not copy `is_user_present` onto the seat or a name, and it does not overwrite seat or guest mode that telemetry already set.

## Trips and telemetry

A trip starts when gear leaves Park and speed is above 0. It ends after gear has been Park for 2 minutes. Honk stores the start, the end, the max speed, the distance, and the polyline, and it calls out a max speed over the limit.

Distance is the polyline's haversine length. If a manual refresh includes an odometer, that delta wins when it looks sane. Odometer is not a streaming field.

Prefer Fleet Telemetry over polling. The subscription fields are `VehicleSpeed`, `Gear`, `Location`, `Soc`, `EstBatteryRange`, `ChargeState`, `DoorState`, `Locked`, `DriverSeatOccupied`, and `GuestModeEnabled`. Speed and location use a 10 second minimum interval. Battery fields use 60 seconds. Gear, doors, lock, seat, and guest mode also use 10 seconds. The car sends a field when it changes, and not more often than the interval. `VehicleSpeed` is miles per hour. `EstBatteryRange` is treated as miles. `VehicleSpeed` includes `minimum_delta` of 1. Drop that key if the firmware rejects the config.

Sample bodies:

- `backend/deploy/telemetry.config.sample.json` is the config the server signs and posts.
- `backend/deploy/fleet-telemetry.server_config.sample.json` is a starting config for [teslamotors/fleet-telemetry](https://github.com/teslamotors/fleet-telemetry).

That official server is a separate TLS process. Its hostname goes in the car config, and its CA goes in `TELEMETRY_CA_FILE`. Vercel and GitHub Pages cannot be that server. The sample turns on `transmit_decoded_records` and the logger. Honk does not embed the websocket server. Point a forwarder at:

```text
POST /ingest/telemetry
POST /ingest/connectivity
Authorization: Bearer $TELEMETRY_INGEST_TOKEN
```

The body can be one dispatcher record, `{"records":[...]}`, or `{"signals":{...}}`. Bad records are skipped. The demo VIN is ignored so a replay cannot clobber the live row.

Sending the config from the Setup sheet is one signed command. The signature is Tesla's Schnorr JWT (`alg` `Tesla.SS256`), made with `github.com/teslamotors/vehicle-command`. It is not a normal ES256 JWT.

## Alerts

The defaults are a 75 mph speed cap, a curfew from 23:00 inclusive through 04:00 exclusive in `America/Los_Angeles`, and a leave-home geofence of 250 meters once a home is set. The phone can replace the timezone. Curfew is evaluated in that zone. A new alert is stored, pushed with APNs when a `.p8` is configured, and shown as a local notification. Missing APNs config does not drop the alert.

## iOS app

The Xcode project is generated with XcodeGen. This Mac has XcodeGen 2.46.0 and only the Command Line Tools, so the app has not been compiled here. Full Xcode is required to build, sign, and archive.

```bash
cd ios
xcodegen generate
open Honk.xcodeproj
```

- Bundle id placeholder: `com.example.honkifyoureskylar`. Change it before you upload anything.
- Deployment target: iOS 17.
- URL scheme: `honkifyoureskylar`.
- API base, `HONKAPIBaseURL` in Info.plist: `http://127.0.0.1:8080`.
- `NSAllowsLocalNetworking` is on. There is no device-location permission. The map uses the car's coordinates from the server.
- Push environment in the entitlement is `development`.
- `ITSAppUsesNonExemptEncryption` is false.
- Same six screens for demo and live: Garage, Map, Trips, Drivers, Alerts, and the Setup sheet.

`127.0.0.1` is the simulator talking to this Mac. A phone on the network needs the Mac's LAN address in `HONKAPIBaseURL`. After the Go server owns `https://skylar.snapcollectibles.com`, put that URL in the plist instead.

Sign in opens `{API base}/auth/tesla/start` with `ASWebAuthenticationSession`. Tesla then redirects to the portal redirect, which is the public `/path`. That hop reaches the Go server only after the domain points at it.

## TestFlight

There is no fastlane project and no App Store Connect API key. Archive from Xcode's Organizer and upload the build yourself. This machine cannot archive. Change the bundle id off `com.example` before upload. The app is for one owner, so internal testing is enough.

## Tests

```bash
export PATH="$HOME/.local/go/bin:$PATH"
cd backend && go test ./...
```

Tests do not call Tesla. `make test` does the same. `make run` starts the server. `make ios` regenerates the Xcode project.
