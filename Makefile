VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/AltairCA/ExitLagFree/internal/version.Version=$(VERSION)
BIN := bin

.PHONY: all test vet node node-linux helper helper-all app app-dev clean

all: test node-linux helper

test:
	go test ./...

vet:
	go vet ./...
	GOOS=linux go vet ./...
	GOOS=windows go vet ./...

node-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN)/exitlag-node-linux-amd64 ./cmd/exitlag-node
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN)/exitlag-node-linux-arm64 ./cmd/exitlag-node

helper:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN)/exitlag-helper$(shell go env GOEXE) ./cmd/exitlag-helper

helper-all:
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN)/exitlag-helper-darwin-arm64 ./cmd/exitlag-helper
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN)/exitlag-helper-darwin-amd64 ./cmd/exitlag-helper
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN)/exitlag-helper-linux-amd64 ./cmd/exitlag-helper
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN)/exitlag-helper-windows-amd64.exe ./cmd/exitlag-helper

# The desktop app needs the Wails CLI: go install github.com/wailsapp/wails/v2/cmd/wails@latest
# The helper binary is bundled next to the app executable (inside the .app on macOS).
# On Windows, also place wintun.dll (https://www.wintun.net) next to the helper.
app: helper
	cd app && wails build -ldflags "$(LDFLAGS)"
ifeq ($(shell go env GOOS),darwin)
	cp $(BIN)/exitlag-helper app/build/bin/ExitLagFree.app/Contents/MacOS/
else
	cp $(BIN)/exitlag-helper$(shell go env GOEXE) app/build/bin/
endif

# In dev mode the app finds the helper in ./bin.
app-dev: helper
	cd app && wails dev

clean:
	rm -rf $(BIN) dist app/build/bin
