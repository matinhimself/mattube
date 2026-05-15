PROXY     = socks5://127.0.0.1:10814
GOPROXY   = https://proxy.golang.org,direct
CLIENT_DIR = client
SERVER_DIR = server
FRONT_DIR  = frontend

# ── Build ────────────────────────────────────────────────────────────────────

.PHONY: build build-server build-client build-frontend

build: build-frontend build-server build-client

build-server:
	cd $(SERVER_DIR) && go build -o ../bin/mattube-server .

build-client: build-frontend
	cd $(CLIENT_DIR) && go build -o ../bin/mattube-client .

build-frontend:
	cd $(FRONT_DIR) && npm run build

# ── Run ──────────────────────────────────────────────────────────────────────

.PHONY: run-server run-client dev

run-server:
	cd $(SERVER_DIR) && go run .

run-client:
	cd $(CLIENT_DIR) && go run .

# Frontend dev server with hot reload (proxies /api → :8080)
dev:
	cd $(FRONT_DIR) && npm run dev

# ── Setup helpers ─────────────────────────────────────────────────────────────
# Run these once before starting the servers.

.PHONY: get-drive-token print-drive-token create-admin create-user list-users test-fronting

# Step 1: OAuth flow — opens browser, saves drive_token.json
# Usage: make get-drive-token CREDS=credentials.json
get-drive-token:
	cd $(CLIENT_DIR) && go run . get-drive-token $(CREDS) $(TOKEN_OUT)

# Print a fresh access token (auto-refreshes if expired)
# Usage: make print-drive-token
print-drive-token:
	cd $(CLIENT_DIR) && go run . print-drive-token

# Create an admin user in the DB
# Usage: make create-admin USER=alice PASS=secret
create-admin:
	cd $(CLIENT_DIR) && go run . create-admin $(USER) $(PASS)

# Create a regular user
# Usage: make create-user USER=bob PASS=secret
create-user:
	cd $(CLIENT_DIR) && go run . create-user $(USER) $(PASS)

# List all users
list-users:
	cd $(CLIENT_DIR) && go run . list-users

# Test SNI fronting connectivity (4 checks)
# Usage: make test-fronting IP=216.239.38.120 SNI=www.google.com
test-fronting:
	cd $(CLIENT_DIR) && go run . test-fronting $(IP) $(SNI)

# Fetch metadata, formats, and related videos for a YouTube video ID
# Usage: make test-video IP=216.239.38.120 SNI=www.google.com VID=6RVGfO45_XM
test-video:
	cd $(CLIENT_DIR) && go run . test-video $(IP) $(SNI) $(VID)

# ── Deps ──────────────────────────────────────────────────────────────────────

.PHONY: deps deps-server deps-client deps-frontend

deps: deps-server deps-client deps-frontend

deps-server:
	cd $(SERVER_DIR) && ALL_PROXY=$(PROXY) GOPROXY=$(GOPROXY) go mod tidy

deps-client:
	cd $(CLIENT_DIR) && ALL_PROXY=$(PROXY) GOPROXY=$(GOPROXY) go mod tidy

deps-frontend:
	cd $(FRONT_DIR) && ALL_PROXY=$(PROXY) npm install
