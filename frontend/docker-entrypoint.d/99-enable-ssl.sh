#!/bin/sh
set -e
if [ -f /etc/nginx/ssl/fullchain.pem ] && [ -f /etc/nginx/ssl/privkey.pem ]; then
  echo "nginx: SSL certs found, switching to HTTPS config (listen 80 + 443)"
  cp /etc/nginx/ssl-config/default-https.conf /etc/nginx/conf.d/default.conf
fi
