#!/bin/sh
set -e

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
