.PHONY: desktop-dev desktop-build desktop-run desktop-frontend-install desktop-frontend-dev desktop-frontend-build test

DESKTOP_DIR := frontend/desktop
DESKTOP_CMD := ./cmd/agentflow-desktop

desktop-dev: desktop-frontend-install
	cd $(DESKTOP_DIR) && npm run build:dev
	go run $(DESKTOP_CMD)

desktop-build: desktop-frontend-install
	cd $(DESKTOP_DIR) && npm run build
	go build $(DESKTOP_CMD)

desktop-run:
	go run $(DESKTOP_CMD)

desktop-frontend-install:
	cd $(DESKTOP_DIR) && npm install

desktop-frontend-dev: desktop-frontend-install
	cd $(DESKTOP_DIR) && npm run dev

desktop-frontend-build: desktop-frontend-install
	cd $(DESKTOP_DIR) && npm run build

test:
	go test ./...
	cd $(DESKTOP_DIR) && npm test
