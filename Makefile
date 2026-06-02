VERSION ?= dev
TAGS    := fts5
LDFLAGS := -X main.Version=$(VERSION)

MODEL_SRC      := build/models/all-MiniLM-L6-v2
# macOS app bundle
MODEL_DST_MAC  := build/bin/Light.app/Contents/Resources/models/all-MiniLM-L6-v2
# Windows：exe 同目录下的 models/ 文件夹
MODEL_DST_WIN  := build/bin/models/all-MiniLM-L6-v2

.PHONY: build build-windows dev install clean copy-models copy-models-windows

build:
	wails build -tags "$(TAGS)" -ldflags "$(LDFLAGS)"
	$(MAKE) copy-models

build-windows:
	wails build -tags "$(TAGS)" -ldflags "$(LDFLAGS)" -platform windows/amd64
	$(MAKE) copy-models-windows

dev:
	wails dev -tags "$(TAGS)"

# macOS：复制进 app bundle
copy-models:
	@if [ -d "$(MODEL_SRC)" ]; then \
		echo "Copying embedding model into app bundle (macOS)..."; \
		mkdir -p "$(MODEL_DST_MAC)"; \
		cp -f "$(MODEL_SRC)"/* "$(MODEL_DST_MAC)/"; \
		echo "Model copied to $(MODEL_DST_MAC)"; \
	else \
		echo "Warning: $(MODEL_SRC) not found, skipping model copy"; \
	fi

# Windows：复制到 exe 同目录下的 models/
copy-models-windows:
	@if [ -d "$(MODEL_SRC)" ]; then \
		echo "Copying embedding model (Windows)..."; \
		mkdir -p "$(MODEL_DST_WIN)"; \
		cp -f "$(MODEL_SRC)"/* "$(MODEL_DST_WIN)/"; \
		echo "Model copied to $(MODEL_DST_WIN)"; \
	else \
		echo "Warning: $(MODEL_SRC) not found, skipping model copy"; \
	fi

install: build
	osascript -e 'do shell script "cp -Rf \"$(PWD)/build/bin/Light.app\" \"/Applications/\"" with administrator privileges'

clean:
	rm -rf build/bin
