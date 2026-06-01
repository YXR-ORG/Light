VERSION ?= dev
TAGS    := fts5
LDFLAGS := -X main.Version=$(VERSION)

.PHONY: build dev install clean

build:
	wails build -tags "$(TAGS)" -ldflags "$(LDFLAGS)"

dev:
	wails dev -tags "$(TAGS)"

install: build
	osascript -e 'do shell script "cp -Rf \"$(PWD)/build/bin/Light.app\" \"/Applications/\"" with administrator privileges'

clean:
	rm -rf build/bin
