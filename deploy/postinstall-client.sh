#!/bin/sh
systemctl daemon-reload
systemctl enable mattube-client
systemctl restart mattube-client
