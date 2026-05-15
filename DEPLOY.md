# Deployment Guide

mattube has two independent binaries that can run on the same machine or separate ones:

- **mattube-client** — web UI + API server; accepts download requests from users and writes job files to Google Drive.
- **mattube-server** — background worker; polls Google Drive for job files, downloads videos via yt-dlp, and uploads results back to Drive.

Both use the same `/etc/mattube/config.json` path by default, so if they share a machine they share one config file.

---

## Prerequisites

- Linux (amd64 or arm64) for production; macOS works for development.
- Go 1.25+ and Node 22+ if building from source.
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) installed and on `$PATH` on the server machine.
- A Google account with Google Drive.
- A GCP project with the **Google Drive API** enabled.

---

## 1. Google OAuth2 Setup

Both the client and server authenticate with Google Drive using OAuth2 user credentials (not a service account).

### 1a. Create OAuth2 credentials

1. Open [console.cloud.google.com/apis/credentials](https://console.cloud.google.com/apis/credentials).
2. Click **Create Credentials → OAuth client ID**.
3. Application type: **Desktop app**.
4. Download the JSON and place it at `/etc/mattube/credentials.json` on each machine.

The file looks like:
```json
{
  "installed": {
    "client_id": "123456.apps.googleusercontent.com",
    "client_secret": "GOCSPX-...",
    ...
  }
}
```

### 1b. Enable the Drive API

APIs & Services → Enable APIs & Services → search **Google Drive API** → Enable.

### 1c. Authorize (run once per machine)

On each machine (client and server), run the OAuth flow once. It opens a browser, asks you to sign in with your Google account, and saves the token.

```bash
# client machine
mattube-client get-drive-token /etc/mattube/credentials.json /etc/mattube/drive_token.json

# server machine
mattube-server get-drive-token /etc/mattube/credentials.json /etc/mattube/drive_token.json
```

The token is saved to `/etc/mattube/drive_token.json` and **auto-refreshed** whenever it expires — you only need to run this once.

> If the machine is headless, the auth URL is printed to stdout. Open it on any browser, complete the flow, and the callback is caught by a local listener on a random port.

---

## 2. Google Drive Folder Setup

You need two Drive folders:

| Folder | Purpose | Config key |
|--------|---------|------------|
| Input folder | Client writes `request-*.json` here; server polls and deletes them | `drive_folder_id` |
| Output folder | Server uploads completed video files here | `drive_output_folder_id` |

Create both folders in your Google Drive, then copy their IDs from the URL:
`https://drive.google.com/drive/folders/<FOLDER_ID>`

---

## 3. Configuration

Both binaries read `/etc/mattube/config.json`. Use a single merged file when running on the same machine, or separate files with `-c`.

```bash
sudo mkdir -p /etc/mattube
sudo cp deploy/config.example.json /etc/mattube/config.json
sudo $EDITOR /etc/mattube/config.json
```

### Full config reference

```json
{
  "fronting_ip":          "216.239.38.120",
  "allowed_sni":          "www.google.com",

  "drive_folder_id":      "YOUR_INPUT_FOLDER_ID",
  "drive_output_folder_id": "YOUR_OUTPUT_FOLDER_ID",

  "credentials_file":     "/etc/mattube/credentials.json",
  "token_file":           "/etc/mattube/drive_token.json",
  "drive_access_token":   "",

  "youtube_api_key":      "",

  "download_dir":         "/tmp/mattube",
  "poll_interval_s":      5,
  "max_concurrent_jobs":  2,
  "https_proxy":          "socks5://127.0.0.1:10814",

  "http_addr":            ":8080",
  "db_path":              "/var/lib/mattube/mattube-client.db",

  "admin_username":       "admin",
  "admin_password":       "changeme"
}
```

| Key | Used by | Default | Description |
|-----|---------|---------|-------------|
| `fronting_ip` | client | — | IP to connect to for domain fronting (Google CDN edge) |
| `allowed_sni` | client | — | SNI hostname presented in TLS handshake |
| `drive_folder_id` | both | — | Drive folder ID polled for `request-*.json` files |
| `drive_output_folder_id` | server | — | Drive folder ID where completed files are uploaded |
| `credentials_file` | both | `/etc/mattube/credentials.json` | OAuth2 Desktop-app credentials from GCP |
| `token_file` | both | `/etc/mattube/drive_token.json` | Saved OAuth2 token (written by `get-drive-token`) |
| `drive_access_token` | both | — | Optional: override token directly instead of loading from file |
| `youtube_api_key` | client | — | Optional YouTube Data API key |
| `download_dir` | server | `/tmp/mattube` | Temporary directory for yt-dlp downloads |
| `poll_interval_s` | server | `5` | How often (seconds) to poll Drive for new jobs |
| `max_concurrent_jobs` | server | `2` | Worker pool size |
| `https_proxy` | server | `socks5://127.0.0.1:10814` | Proxy passed to yt-dlp via `HTTPS_PROXY` |
| `http_addr` | client | `:8080` | Address the web server listens on |
| `db_path` | client | `/var/lib/mattube/mattube-client.db` | SQLite database path |
| `admin_username` | client | — | Bootstrap admin created on first start (if no users exist) |
| `admin_password` | client | — | Bootstrap admin password — change after first login |

---

## 4. Installation

### Option A — Pre-built .deb packages (recommended)

Download the latest `.deb` from the [GitHub Releases](https://github.com/matinhimself/mattube/releases) page.

```bash
sudo dpkg -i mattube-client_<version>_amd64.deb
sudo dpkg -i mattube-server_<version>_amd64.deb
```

The packages install binaries to `/usr/bin/` and register systemd services. They do **not** create the config file — do that in step 3 first.

### Option B — Build from source

```bash
git clone https://github.com/matinhimself/mattube
cd mattube
make build          # builds bin/mattube-client and bin/mattube-server

sudo cp bin/mattube-client /usr/bin/
sudo cp bin/mattube-server /usr/bin/
sudo cp deploy/mattube-client.service /lib/systemd/system/
sudo cp deploy/mattube-server.service /lib/systemd/system/
sudo systemctl daemon-reload
```

---

## 5. System User and Directories

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin mattube
sudo mkdir -p /var/lib/mattube /opt/mattube
sudo chown mattube:mattube /var/lib/mattube /opt/mattube
sudo chown mattube:mattube /etc/mattube/drive_token.json   # server writes refreshed tokens here
```

---

## 6. Create the First Admin User

On first start the client auto-creates an admin from `admin_username`/`admin_password` in config if the database is empty. Alternatively, create one explicitly before starting:

```bash
mattube-client -c /etc/mattube/config.json create-admin alice secretpassword
```

After logging in, change your password in the UI and remove `admin_username`/`admin_password` from the config.

---

## 7. Start and Enable Services

```bash
sudo systemctl enable --now mattube-client
sudo systemctl enable --now mattube-server

# Check status
sudo systemctl status mattube-client
sudo systemctl status mattube-server

# Follow logs
sudo journalctl -fu mattube-client
sudo journalctl -fu mattube-server
```

---

## 8. Fronting IP

`fronting_ip` is a Google CDN edge IP. The default `216.239.38.120` works in most regions. To find alternatives:

```bash
mattube-client test-fronting 216.239.38.120 www.google.com
```

Try other IPs from the `216.239.32.0/19` range if the default is blocked.

---

## 9. CLI Reference

### mattube-client

```
mattube-client [-c config.json] <command>

  serve                                   Start the web server (default)
  create-admin  <username> <password>     Create an admin user
  create-user   <username> <password>     Create a regular user
  list-users                              List all users
  get-drive-token [creds.json] [out.json] OAuth flow — saves Drive token
  print-drive-token [creds.json] [tok.json] Print a fresh access token
  test-fronting  <ip> <sni>              Test SNI fronting connectivity
  test-video     <ip> <sni> <video-id>   Fetch metadata and formats for a video
```

### mattube-server

```
mattube-server [-c config.json] <command>

  serve                                   Start the server (default)
  get-drive-token [creds.json] [out.json] OAuth flow — saves Drive token
  print-drive-token [creds.json] [tok.json] Print a fresh access token
```

---

## 10. Upgrading

```bash
# .deb
sudo dpkg -i mattube-client_<new_version>_amd64.deb
sudo dpkg -i mattube-server_<new_version>_amd64.deb
sudo systemctl restart mattube-client mattube-server

# from source
make build
sudo cp bin/mattube-client bin/mattube-server /usr/bin/
sudo systemctl restart mattube-client mattube-server
```

The SQLite database and token files are not touched on upgrade.
