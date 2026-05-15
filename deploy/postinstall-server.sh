#!/bin/sh
systemctl daemon-reload
systemctl enable mattube-server
systemctl restart mattube-server
