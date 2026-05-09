.PHONY: all build build-macos-helper build-macos clean install test

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
MACOSX_DEPLOYMENT_TARGET ?= 26.0

all: build

build:
	go build -o calbar ./cmd/calbar

build-macos-helper:
	MACOSX_DEPLOYMENT_TARGET=$(MACOSX_DEPLOYMENT_TARGET) xcrun swiftc -O cmd/calbar-macos-helper/main.swift -o calbar-macos-helper

build-macos: build build-macos-helper

clean:
	rm -f calbar calbar-macos-helper

install: build
	install -Dm755 calbar $(DESTDIR)$(BINDIR)/calbar
	install -Dm644 configs/calbar.service $(DESTDIR)$(PREFIX)/lib/systemd/user/calbar.service

install-user: build
	mkdir -p ~/.local/bin
	cp calbar ~/.local/bin/
	mkdir -p ~/.config/systemd/user
	cp configs/calbar.service ~/.config/systemd/user/
	@echo "Installed to ~/.local/bin"
	@echo "Run: systemctl --user enable --now calbar"

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/calbar
	rm -f $(DESTDIR)$(PREFIX)/lib/systemd/user/calbar.service

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	go vet ./...
