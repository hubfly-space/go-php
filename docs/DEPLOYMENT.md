# Deployment Guide

This guide covers production deployment of go-php-gateway.

## Prerequisites

- Linux server (Ubuntu 22.04+, Debian 12+, or RHEL 9+)
- Go 1.23+ (for building)
- PHP-FPM 8.0+
- Systemd (for service management)
- TLS certificates (for HTTPS)

## Build

```bash
# Build with version info
go build -ldflags "-X main.version=$(git describe --tags)" \
  -o gateway ./cmd/gateway

# Verify
./gateway --version
```

## System Service

### Create Service File

Create `/etc/systemd/system/gateway.service`:

```ini
[Unit]
Description=Go-PHP Gateway
After=network.target php8.3-fpm.service
Requires=php8.3-fpm.service

[Service]
Type=simple
User=www-data
Group=www-data
ExecStart=/opt/gateway/gateway serve /var/www/app --config /etc/gateway/gateway.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5
LimitNOFILE=65536
WorkingDirectory=/var/www/app

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/gateway /var/cache/gateway
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

### Enable and Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable gateway
sudo systemctl start gateway
sudo systemctl status gateway
```

## Configuration

### Production Config

```yaml
schema: gateway/v1

server:
  addr: "0.0.0.0:8080"
  read_timeout: 30s
  write_timeout: 60s
  read_header_timeout: 5s
  idle_timeout: 120s
  max_header_bytes: 1048576

php:
  binary: /usr/sbin/php-fpm
  max_children: 20
  start_servers: 2
  min_spare: 2
  max_spare: 6
  max_requests: 500
  request_timeout: 60s

routes:
  - path_prefix: /api/
    target: /index.php
    methods: [GET, POST, PUT, DELETE]
  - path_prefix: /
    target: /index.php

security:
  symlink_mode: within_root
  max_body_size: 20MB
  protected_patterns:
    - .env
    - .git
    - "*.sql"
    - "*.log"

logging:
  format: json
  level: info
```

### TLS Configuration

```yaml
tls:
  enabled: true
  cert_file: /etc/ssl/certs/gateway.pem
  key_file: /etc/ssl/private/gateway.key
  min_version: "1.2"
```

### ACME (Let's Encrypt)

```yaml
tls:
  enabled: true
  acme:
    enabled: true
    email: admin@example.com
    domains:
      - example.com
      - www.example.com
    storage: /var/lib/gateway/acme
```

## Reverse Proxy Setup

### Nginx

```nginx
upstream gateway {
    server 127.0.0.1:8080;
}

server {
    listen 80;
    server_name example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name example.com;

    ssl_certificate /etc/ssl/certs/example.pem;
    ssl_certificate_key /etc/ssl/private/example.key;

    location / {
        proxy_pass http://gateway;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### HAProxy

```
frontend https
    bind *:443 ssl crt /etc/ssl/certs/example.pem
    default_backend gateway

backend gateway
    server gateway 127.0.0.1:8080 check
```

## Zero-Downtime Deployment

### Create Release

```bash
# Create immutable release
gateway deploy create v1.0.0

# Deploy with canary (10% traffic)
gateway deploy canary v1.0.0 --weight 10

# Promote to 100%
gateway deploy promote v1.0.0

# Rollback if needed
gateway deploy rollback
```

### Deployment Hooks

Create pre/post deployment scripts:

```bash
# /etc/gateway/hooks/pre-deploy.sh
#!/bin/bash
php /var/www/app/artisan migrate --force

# /etc/gateway/hooks/post-deploy.sh
#!/bin/bash
php /var/www/app/artisan cache:clear
```

## Monitoring

### Health Check

```bash
curl http://localhost:8080/api/health
```

### Metrics

Prometheus endpoint at `/api/metrics`:

```
# HELP gateway_requests_total Total HTTP requests
# TYPE gateway_requests_total counter
gateway_requests_total{method="GET",status="200"} 12345

# HELP gateway_request_duration_seconds Request duration
# TYPE gateway_request_duration_seconds histogram
gateway_request_duration_seconds_bucket{le="0.1"} 12000
```

### Logs

```bash
# View structured logs
journalctl -u gateway -f

# Check for errors
journalctl -u gateway -p err
```

### Audit

```bash
# View audit logs
gateway status --audit
```

## Performance Tuning

### PHP-FPM Pool

```ini
; /etc/php/8.3/fpm/pool.d/www.conf
[www]
pm = dynamic
pm.max_children = 20
pm.start_servers = 2
pm.min_spare_servers = 2
pm.max_spare_servers = 6
pm.max_requests = 500
```

### OS Tuning

```bash
# Increase file descriptors
echo "* soft nofile 65536" >> /etc/security/limits.conf
echo "* hard nofile 65536" >> /etc/security/limits.conf

# Increase TCP connections
echo "net.core.somaxconn = 65535" >> /etc/sysctl.conf
sysctl -p
```

## Backup and Recovery

### State Backup

```bash
# Backup state
cp /var/lib/gateway/state.json /backup/gateway-state-$(date +%s).json

# Restore state
cp /backup/gateway-state-1234567890.json /var/lib/gateway/state.json
systemctl restart gateway
```

### Incident Snapshot

```bash
# Create incident snapshot
gateway snapshot create "production incident 2024-01-15"

# List snapshots
gateway snapshot list
```

## Troubleshooting

### Common Issues

**FPM connection refused:**
```bash
# Check FPM status
systemctl status php8.3-fpm

# Check socket
ls -la /run/php/php8.3-fpm.sock
```

**Permission denied:**
```bash
# Add gateway user to FPM group
sudo usermod -a -G www-data gateway
```

**High memory usage:**
```bash
# Reduce FPM children
# Edit /etc/php/8.3/fpm/pool.d/www.conf
pm.max_children = 10
```

### Debug Mode

```bash
# Run with debug logging
gateway serve . --config gateway.yaml --log-level debug

# Check doctor output
gateway doctor
```

## Security Checklist

- [ ] TLS enabled with strong ciphers
- [ ] Protected files configured (`.env`, `.git`, etc.)
- [ ] Rate limiting enabled
- [ ] Admin API bound to localhost only
- [ ] Audit logging enabled
- [ ] Secrets not in config files
- [ ] FPM user has minimal privileges
- [ ] Systemd security hardening applied
