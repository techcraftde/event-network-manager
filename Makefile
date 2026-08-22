.PHONY: dev-backend dev-real dev-frontend test build

GO_ENV := GOPATH=$(CURDIR)/work/go GOCACHE=$(CURDIR)/work/go-build GOMODCACHE=$(CURDIR)/work/go-mod

dev-backend:
	cd backend && $(GO_ENV) go run ./cmd/enm-server

dev-real:
	@test -n "$$ENM_SWITCH_ADDRESS" -a -n "$$ENM_SWITCH_USERNAME" -a -n "$$ENM_SWITCH_PASSWORD" || (echo "ENM_SWITCH_ADDRESS, ENM_SWITCH_USERNAME and ENM_SWITCH_PASSWORD are required"; exit 1)
	cd backend && $(GO_ENV) go run ./cmd/enm-server

dev-frontend:
	cd frontend && npm install --cache ../work/npm-cache && npm run dev

test:
	cd backend && $(GO_ENV) go test ./...
	cd frontend && npm install --cache ../work/npm-cache && npm run typecheck

build:
	cd frontend && npm install --cache ../work/npm-cache && npm run build
	cd backend && $(GO_ENV) go build ./cmd/enm-server
