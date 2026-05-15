#!/bin/sh
set -e

systemctl daemon-reload
systemctl enable mattube-client

if systemctl is-active --quiet mattube-client; then
    systemctl restart mattube-client
    echo ""
    echo "mattube-client restarted."
else
    echo ""
    echo "================================================"
    echo "  mattube-client installed"
    echo "================================================"
    echo ""
    echo "Before starting, edit /etc/mattube/config.json:"
    echo ""
    echo "  fronting_ip       Google CDN edge IP (e.g. 216.239.38.120)"
    echo "  allowed_sni       SNI hostname        (e.g. www.google.com)"
    echo "  drive_folder_id   Google Drive folder ID for job requests"
    echo "  admin_username    Bootstrap admin username"
    echo "  admin_password    Bootstrap admin password"
    echo ""
    echo "Then authorize Google Drive access (opens browser):"
    echo ""
    echo "  mattube-client get-drive-token /etc/mattube/credentials.json /etc/mattube/drive_token.json"
    echo ""
    echo "Then start the service:"
    echo ""
    echo "  systemctl start mattube-client"
    echo ""
fi
