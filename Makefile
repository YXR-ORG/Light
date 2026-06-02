VERSION ?= dev
TAGS    := fts5
LDFLAGS := -X main.Version=$(VERSION)

MODEL_SRC := build/models/all-MiniLM-L6-v2
MODEL_DST := build/bin/Light.app/Contents/Resources/models/all-MiniLM-L6-v2

.PHONY: build dev install clean copy-models

build:
	wails build -tags "$(TAGS)" -ldflags "$(LDFLAGS)"
	$(MAKE) copy-models

dev:
	wails dev -tags "$(TAGS)"

# 把 embedding 模型文件复制进 app bundle
copy-models:
	@if [ -d "$(MODEL_SRC)" ]; then \
		echo "Copying embedding model into app bundle..."; \
		mkdir -p "$(MODEL_DST)"; \
		cp -f "$(MODEL_SRC)"/* "$(MODEL_DST)/"; \
		echo "Model copied to $(MODEL_DST)"; \
	else \
		echo "Warning: $(MODEL_SRC) not found, skipping model copy"; \
	fi

install: build
	osascript -e 'do shell script "cp -Rf \"$(PWD)/build/bin/Light.app\" \"/Applications/\"" with administrator privileges'

clean:
	rm -rf build/bin
