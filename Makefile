# Prohod build. The engine (third_party/byedpi, upstream hufrea/byedpi @ e268477, MIT) is compiled natively
# and embedded into the Go binary.
UNAME_S := $(shell uname -s | tr A-Z a-z)
UNAME_M := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
ENGINE  := internal/engine/bin/core-$(UNAME_S)-$(UNAME_M)
WAILS   ?= $(HOME)/go/bin/wails

.PHONY: engine test app dev clean

engine: $(ENGINE)

$(ENGINE): $(wildcard third_party/byedpi/*.c third_party/byedpi/*.h)
	$(MAKE) -C third_party/byedpi clean >/dev/null
	$(MAKE) -C third_party/byedpi CFLAGS="-O2 -Wall -Wno-unused -std=c99 -I." >/dev/null
	strip third_party/byedpi/ciadpi
	install -m 0755 third_party/byedpi/ciadpi $(ENGINE)
	$(MAKE) -C third_party/byedpi clean >/dev/null

test: engine
	go test ./internal/...

app: engine
	$(WAILS) build -clean -trimpath -ldflags "-s -w" $(if $(filter linux,$(UNAME_S)),-tags webkit2_41,)

dev: engine
	$(WAILS) dev $(if $(filter linux,$(UNAME_S)),-tags webkit2_41,)

clean:
	rm -rf build/bin internal/engine/bin/core-*
