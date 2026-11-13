# Build, package and install lodev. Run `make help`.
VERSION ?= $(shell git describe --tags --match 'v*' --always --dirty 2>/dev/null || echo dev)
VER      = $(VERSION:v%=%)
CHANNEL ?=
REPO    ?= namnh198/lodev
PREFIX  ?= /usr/local
TARGETS ?= linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

HOST    := $(shell go env GOOS)_$(shell go env GOARCH)
HOSTBIN  = dist/bin/$(HOST)/lodev
LDFLAGS  = -s -w -X lodev/cmd.Version=$(VERSION) -X lodev/cmd.Channel=$(CHANNEL)
LINUX_ARCHES = $(patsubst linux/%,%,$(filter linux/%,$(TARGETS)))

.PHONY: build completions dist deb packaging packages install uninstall test clean help

.DEFAULT_GOAL := completions

## build: cross-build every target into dist/bin/<os>_<arch>/
build:
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "build $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" \
			-o dist/bin/$${os}_$$arch/lodev$$ext . || exit 1; \
	done

## completions: build, then bash/zsh/fish completion scripts into dist/completions/ (default)
completions: build
	@mkdir -p dist/completions
	$(HOSTBIN) completion bash > dist/completions/lodev.bash
	$(HOSTBIN) completion zsh > dist/completions/_lodev
	$(HOSTBIN) completion fish > dist/completions/lodev.fish

## dist: one archive per target plus dist/checksums.txt
dist: completions
	@rm -rf dist/stage dist/*.tar.gz dist/*.zip dist/checksums.txt
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; name=lodev_$(VER)_$${os}_$$arch; dir=dist/stage/$$name; \
		mkdir -p $$dir/completions && cp dist/bin/$${os}_$$arch/* $$dir/ && \
		cp dist/completions/* $$dir/completions/ && cp README.md LICENSE $$dir/ || exit 1; \
		if [ $$os = windows ]; then (cd dist/stage && zip -qr ../$$name.zip $$name); \
		else COPYFILE_DISABLE=1 tar --no-xattrs -czf dist/$$name.tar.gz -C dist/stage $$name; fi || exit 1; \
		echo "dist  $$name"; \
	done
	@cd dist && shasum -a 256 *.tar.gz *.zip > checksums.txt
	@rm -rf dist/stage

## deb: lodev_<ver>_<arch>.deb for each Linux target
deb: completions
	@for arch in $(LINUX_ARCHES); do \
		go run ./tools/mkdeb -version $(VER) -arch $$arch -bin dist/bin/linux_$$arch/lodev \
			-completions dist/completions -license LICENSE -out dist/lodev_$(VER)_$$arch.deb || exit 1; \
		echo "deb   dist/lodev_$(VER)_$$arch.deb"; \
	done

## packaging: Homebrew formula, AUR PKGBUILD/.SRCINFO into dist/packaging/ (needs dist)
packaging:
	go run ./tools/pkggen -version $(VER) -repo $(REPO) -checksums dist/checksums.txt \
		-templates packaging -out dist/packaging

## packages: dist + deb + packaging (what a release uploads)
packages: dist deb packaging

## install: install this machine's binary and completions under PREFIX (run `make` first, as yourself)
install:
	@test -x $(HOSTBIN) && test -f dist/completions/lodev.bash || \
		{ echo "nothing to install: run \`make\` first (then \`sudo make install\`)"; exit 1; }
	install -d $(DESTDIR)$(PREFIX)/bin $(DESTDIR)$(PREFIX)/share/bash-completion/completions \
		$(DESTDIR)$(PREFIX)/share/zsh/site-functions $(DESTDIR)$(PREFIX)/share/fish/vendor_completions.d \
		$(DESTDIR)$(PREFIX)/share/doc/lodev
	install -m 0755 $(HOSTBIN) $(DESTDIR)$(PREFIX)/bin/lodev
	install -m 0644 dist/completions/lodev.bash $(DESTDIR)$(PREFIX)/share/bash-completion/completions/lodev
	install -m 0644 dist/completions/_lodev $(DESTDIR)$(PREFIX)/share/zsh/site-functions/_lodev
	install -m 0644 dist/completions/lodev.fish $(DESTDIR)$(PREFIX)/share/fish/vendor_completions.d/lodev.fish
	install -m 0644 LICENSE $(DESTDIR)$(PREFIX)/share/doc/lodev/LICENSE

## uninstall: remove what install put under PREFIX
uninstall:
	rm -f $(DESTDIR)$(PREFIX)/bin/lodev \
		$(DESTDIR)$(PREFIX)/share/bash-completion/completions/lodev \
		$(DESTDIR)$(PREFIX)/share/zsh/site-functions/_lodev \
		$(DESTDIR)$(PREFIX)/share/fish/vendor_completions.d/lodev.fish \
		$(DESTDIR)$(PREFIX)/share/doc/lodev/LICENSE
	-rmdir $(DESTDIR)$(PREFIX)/share/doc/lodev 2>/dev/null

## test: go vet + go test
test:
	go vet ./...
	go test ./...

## clean: remove dist/
clean:
	rm -rf dist

## help: list targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
