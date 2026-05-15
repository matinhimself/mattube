#!/bin/sh
systemctl stop mattube-client mattube-server 2>/dev/null || true
systemctl disable mattube-client mattube-server 2>/dev/null || true
