# YALangSwitcher build. Cross compiles from Linux/WSL, no CGO, no external
# tools except go-winres (only needed to embed the icon + manifest).

APP     := YALangSwitcher
VERSION ?= 0.1.0
OUT     := build/$(APP).exe
LDFLAGS := -s -w -H windowsgui -X main.version=$(VERSION)

.PHONY: all icon resources build vet test clean

all: build

# Regenerates assets/*.ico from the renderer in internal/icon.
icon:
	go run ./tools/genicon

# Icon + version info + "asInvoker" manifest compiled into the exe.
resources:
	GOARCH=amd64 go-winres simply --arch amd64 --manifest gui \
	  --icon assets/app.ico \
	  --product-name YALangSwitcher \
	  --file-description "YALangSwitcher - yet another language switcher" \
	  --copyright "MIT License" \
	  --product-version $(VERSION) --file-version $(VERSION)

build: resources
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(OUT) .
	@ls -la $(OUT)

vet:
	GOOS=windows GOARCH=amd64 go vet -unsafeptr=false ./...

test: vet
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /dev/null .
	gofmt -l .

clean:
	rm -rf build rsrc_windows_*.syso assets/preview
