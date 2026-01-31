.PHONY: all build clean install test

PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin

all: build

build:
	go build -o calsync ./cmd/calsync
	go build -o calbar ./cmd/calbar

clean:
	rm -f calsync calbar

install: build
	install -Dm755 calsync $(DESTDIR)$(BINDIR)/calsync
	install -Dm755 calbar $(DESTDIR)$(BINDIR)/calbar
	install -Dm644 configs/calsync.service $(DESTDIR)$(PREFIX)/lib/systemd/user/calsync.service
	install -Dm644 configs/calbar.service $(DESTDIR)$(PREFIX)/lib/systemd/user/calbar.service

install-user: build
	mkdir -p ~/.local/bin
	cp calsync calbar ~/.local/bin/
	mkdir -p ~/.config/systemd/user
	cp configs/calsync.service configs/calbar.service ~/.config/systemd/user/
	@echo "Installed to ~/.local/bin"
	@echo "Run: systemctl --user enable --now calsync calbar"

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/calsync
	rm -f $(DESTDIR)$(BINDIR)/calbar
	rm -f $(DESTDIR)$(PREFIX)/lib/systemd/user/calsync.service
	rm -f $(DESTDIR)$(PREFIX)/lib/systemd/user/calbar.service

test:
	go test ./...

fmt:
	go fmt ./...

lint:
	go vet ./...
