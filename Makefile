# Version reported by the binary, in preference order: an explicit
# VERSION=, the current git description, or the default compiled into
# app.go. When none of the first two are available the version is left
# alone rather than overridden with an empty string. `git describe` fails
# whenever the build context has no usable git repository, which includes
# a container build from a git worktree or a shallow CI checkout, and that
# produced binaries reporting a bare "v".
VERSION ?=
GITREV := $(shell git describe --tags 2>/dev/null | cut -c 2-)
VERSTR := $(if $(VERSION),$(VERSION),$(GITREV))
VERFLAG := $(if $(VERSTR),-X 'github.com/writefreely/writefreely.softwareVer=$(VERSTR)',)

# Release archives are named after the version, so they need a value even
# when neither VERSION nor git can supply one. Fall back to the constant
# the binary itself would report, which keeps the file name meaningful
# rather than producing writefreely__linux_amd64.tar.gz.
DEFAULTVER := $(shell sed -n 's/^[[:space:]]*softwareVer = "\(.*\)"/\1/p' app.go)
ARCHIVEVER := $(if $(VERSTR),$(VERSTR),$(DEFAULTVER))

LDFLAGS=-ldflags="-s -w $(VERFLAG) -extldflags '-static'"
BASELDFLAGS=-ldflags="-s -w $(VERFLAG)"

GOCMD=go
GOINSTALL=$(GOCMD) install $(LDFLAGS)
GOBUILD=$(GOCMD) build $(LDFLAGS)
GOTEST=$(GOCMD) test $(LDFLAGS)
GOGET=$(GOCMD) get
BINARY_NAME=writefreely
BUILDPATH=build/$(BINARY_NAME)
DOCKERCMD=docker
IMAGE_NAME=ghcr.io/josephquigley/wispwriter
TMPBIN=./tmp

all : build

ci: deps
	cd cmd/writefreely; $(GOBUILD) -v

build: deps
	cd cmd/writefreely; $(GOBUILD) -v -tags='netgo sqlite'

build-no-sqlite: deps-no-sqlite
	cd cmd/writefreely; $(GOBUILD) -v -tags='netgo' -o $(BINARY_NAME)

build-linux: deps
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GOCMD) install src.techknowlogick.com/xgo@latest; \
	fi
	xgo --targets=linux/amd64, -dest build/ $(LDFLAGS) -tags='netgo sqlite' -go go-1.25.x -out writefreely -pkg ./cmd/writefreely .

build-windows: deps
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GOCMD) install src.techknowlogick.com/xgo@latest; \
	fi
	xgo --targets=windows/amd64, -dest build/ $(LDFLAGS) -tags='netgo sqlite' -go go-1.25.x -out writefreely -pkg ./cmd/writefreely .

build-darwin: deps
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GOCMD) install src.techknowlogick.com/xgo@latest; \
	fi
	xgo --targets=darwin/amd64, -dest build/ $(BASELDFLAGS) -tags='netgo sqlite' -go go-1.25.x -out writefreely -pkg ./cmd/writefreely .

build-darwin-arm64: deps
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GOCMD) install src.techknowlogick.com/xgo@latest; \
	fi
	xgo --targets=darwin/arm64, -dest build/ $(BASELDFLAGS) -tags='netgo sqlite' -go go-1.25.x -out writefreely -pkg ./cmd/writefreely .

build-arm6: deps
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GOCMD) install src.techknowlogick.com/xgo@latest; \
	fi
	xgo --targets=linux/arm-6, -dest build/ $(LDFLAGS) -tags='netgo sqlite' -go go-1.25.x -out writefreely -pkg ./cmd/writefreely .

build-arm7: deps
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GOCMD) install src.techknowlogick.com/xgo@latest; \
	fi
	xgo --targets=linux/arm-7, -dest build/ $(LDFLAGS) -tags='netgo sqlite' -go go-1.25.x -out writefreely -pkg ./cmd/writefreely .

build-arm64: deps
	@hash xgo > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GOCMD) install src.techknowlogick.com/xgo@latest; \
	fi
	xgo --targets=linux/arm64, -dest build/ $(LDFLAGS) -tags='netgo sqlite' -go go-1.25.x -out writefreely -pkg ./cmd/writefreely .

build-docker :
	$(DOCKERCMD) build --build-arg WRITEFREELY_VERSION=$(VERSTR) -t $(IMAGE_NAME):latest $(if $(VERSTR),-t $(IMAGE_NAME):$(VERSTR),) .

# Bump the version compiled into the binary, commit it and tag it, so the
# tag and the constant can never disagree. The tag is what CI turns into
# published image tags.
#
#   make bump VERSION=0.18.1
#
# Named bump rather than release because release already builds the
# cross-compiled binary tarballs.
bump:
	@if [ -z "$(VERSION)" ]; then echo "usage: make bump VERSION=x.y.z"; exit 1; fi
	@echo "$(VERSION)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "VERSION must look like x.y.z"; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "working tree is dirty; commit or stash first"; exit 1; }
	@if git rev-parse -q --verify "refs/tags/v$(VERSION)" >/dev/null; then echo "tag v$(VERSION) already exists"; exit 1; fi
	@sed -i.relbak -E 's/^([[:space:]]*softwareVer = ")[^"]*(")/\1$(VERSION)\2/' app.go && rm -f app.go.relbak
	@grep -q 'softwareVer = "$(VERSION)"' app.go || { echo "failed to update softwareVer in app.go"; exit 1; }
	@gofmt -l app.go | grep -q . && { echo "app.go is not gofmt-clean after the edit"; exit 1; } || true
	git add app.go
	git commit -m "Release $(VERSION)"
	git tag -a "v$(VERSION)" -m "Wisp Edition $(VERSION)"
	@echo
	@echo "Tagged v$(VERSION). Publish with:"
	@echo "    git push origin HEAD --tags"

test:
	$(GOTEST) -v ./...

run:
	$(GOINSTALL) -tags='netgo sqlite' ./...
	$(BINARY_NAME) --debug

deps :
	$(GOGET) -tags='sqlite' -d -v ./...

deps-no-sqlite:
	$(GOGET) -d -v ./...

install : build
	cmd/writefreely/$(BINARY_NAME) --config
	cmd/writefreely/$(BINARY_NAME) --gen-keys
	cmd/writefreely/$(BINARY_NAME) --init-db
	cd less/; $(MAKE) install $(MFLAGS)

release : clean ui
	mkdir -p $(BUILDPATH)
	rsync -av --exclude=".*" templates $(BUILDPATH)
	rsync -av --exclude=".*" pages $(BUILDPATH)
	rsync -av --exclude=".*" static $(BUILDPATH)
	rm -r $(BUILDPATH)/static/local
	scripts/invalidate-css.sh $(BUILDPATH)
	mkdir $(BUILDPATH)/keys
	$(MAKE) build-linux
	mv build/$(BINARY_NAME)-linux-amd64 $(BUILDPATH)/$(BINARY_NAME)
	tar -cvzf $(BINARY_NAME)_$(ARCHIVEVER)_linux_amd64.tar.gz -C build $(BINARY_NAME)
	rm $(BUILDPATH)/$(BINARY_NAME)
	$(MAKE) build-arm6
	mv build/$(BINARY_NAME)-linux-arm-6 $(BUILDPATH)/$(BINARY_NAME)
	tar -cvzf $(BINARY_NAME)_$(ARCHIVEVER)_linux_arm6.tar.gz -C build $(BINARY_NAME)
	rm $(BUILDPATH)/$(BINARY_NAME)
	$(MAKE) build-arm7
	mv build/$(BINARY_NAME)-linux-arm-7 $(BUILDPATH)/$(BINARY_NAME)
	tar -cvzf $(BINARY_NAME)_$(ARCHIVEVER)_linux_arm7.tar.gz -C build $(BINARY_NAME)
	rm $(BUILDPATH)/$(BINARY_NAME)
	$(MAKE) build-arm64
	mv build/$(BINARY_NAME)-linux-arm64 $(BUILDPATH)/$(BINARY_NAME)
	tar -cvzf $(BINARY_NAME)_$(ARCHIVEVER)_linux_arm64.tar.gz -C build $(BINARY_NAME)
	rm $(BUILDPATH)/$(BINARY_NAME)
	$(MAKE) build-darwin
	mv build/$(BINARY_NAME)-darwin-10.12-amd64 $(BUILDPATH)/$(BINARY_NAME)
	tar -cvzf $(BINARY_NAME)_$(ARCHIVEVER)_macos_amd64.tar.gz -C build $(BINARY_NAME)
	rm $(BUILDPATH)/$(BINARY_NAME)
	$(MAKE) build-darwin-arm64
	mv build/$(BINARY_NAME)-darwin-10.12-arm64 $(BUILDPATH)/$(BINARY_NAME)
	tar -cvzf $(BINARY_NAME)_$(ARCHIVEVER)_macos_arm64.tar.gz -C build $(BINARY_NAME)
	rm $(BUILDPATH)/$(BINARY_NAME)
	$(MAKE) build-windows
	mv build/$(BINARY_NAME)-windows-4.0-amd64.exe $(BUILDPATH)/$(BINARY_NAME).exe
	cd build; zip -r ../$(BINARY_NAME)_$(ARCHIVEVER)_windows_amd64.zip ./$(BINARY_NAME)
	rm $(BUILDPATH)/$(BINARY_NAME).exe

# This assumes you're on linux/amd64
release-linux : clean ui
	mkdir -p $(BUILDPATH)
	cp -r templates $(BUILDPATH)
	cp -r pages $(BUILDPATH)
	cp -r static $(BUILDPATH)
	mkdir $(BUILDPATH)/keys
	$(MAKE) build-no-sqlite
	mv cmd/writefreely/$(BINARY_NAME) $(BUILDPATH)/$(BINARY_NAME)
	tar -cvzf $(BINARY_NAME)_$(ARCHIVEVER)_linux_amd64.tar.gz -C build $(BINARY_NAME)

release-docker :
	$(DOCKERCMD) push $(IMAGE_NAME)

ui : force_look
	cd less/; $(MAKE) $(MFLAGS)
	cd prose/; $(MAKE) $(MFLAGS)

$(TMPBIN):
	mkdir -p $(TMPBIN)

$(TMPBIN)/xgo: deps $(TMPBIN)
	$(GOBUILD) -o $(TMPBIN)/xgo src.techknowlogick.com/xgo

clean :
	-rm -rf build
	-rm -rf tmp
	cd less/; $(MAKE) clean $(MFLAGS)

force_look :
	true
