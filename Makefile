export PATH := $(HOME)/.local/go/bin:$(PATH)

.PHONY: test run ios

test:
	cd backend && go test ./...

run:
	cd backend && go run ./cmd/honk-server

ios:
	cd ios && xcodegen generate
