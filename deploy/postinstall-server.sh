#!/bin/sh
set -e

# Create system user
if ! id mattube > /dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin mattube
fi

# Create required directories
mkdir -p /etc/mattube /opt/mattube /tmp/mattube

# Fix ownership so the service can read config and write downloads
chown mattube:mattube /opt/mattube /tmp/mattube
if [ -f /etc/mattube/credentials.json ]; then
    chown mattube:mattube /etc/mattube/credentials.json
fi
if [ -f /etc/mattube/drive_token.json ]; then
    chown mattube:mattube /etc/mattube/drive_token.json
fi
if [ -f /etc/mattube/config.json ]; then
    chown mattube:mattube /etc/mattube/config.json
fi

systemctl daemon-reload
systemctl enable mattube-server

if systemctl is-active --quiet mattube-server; then
    systemctl restart mattube-server
    echo ""
    echo "mattube-server restarted."
else
    echo ""
    echo "================================================"
    echo "  mattube-server installed"
    echo "================================================"
    echo ""
    echo "Before starting, edit /etc/mattube/config.json:"
    echo ""
    echo "  drive_folder_id        Google Drive folder ID to poll for jobs"
    echo "  drive_output_folder_id Google Drive folder ID for uploaded results"
    echo "  https_proxy            Proxy for yt-dlp (e.g. socks5://127.0.0.1:10814)"
    echo "  max_concurrent_jobs    Worker pool size (default: 2)"
    echo ""
    echo "Then authorize Google Drive access (opens browser):"
    echo ""
    echo "  mattube-server get-drive-token /etc/mattube/credentials.json /etc/mattube/drive_token.json"
    echo ""
    echo "Then start the service:"
    echo ""
    echo "  systemctl start mattube-server"
    echo ""
fi
