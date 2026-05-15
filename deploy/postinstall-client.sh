#!/bin/sh
set -e

# Create system user
if ! id mattube > /dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin mattube
fi

# Create required directories
mkdir -p /etc/mattube /opt/mattube /var/lib/mattube

# Fix ownership so the service can read config and write DB
chown mattube:mattube /opt/mattube /var/lib/mattube
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
