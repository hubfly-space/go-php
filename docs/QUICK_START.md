# Quick Start Guide

This guide walks you through setting up go-php-gateway to serve a PHP application.

## Prerequisites

1. **Go 1.23+**
2. **PHP-FPM 8.0+** with a working socket or TCP listener
3. A PHP application (e.g., Laravel, Symfony, WordPress)

## Step 1: Install

```bash
# Clone the repository
git clone https://github.com/go-php/gateway.git
cd gateway

# Build
go build -o gateway ./cmd/gateway

# Verify
./gateway --help
```

## Step 2: Configure PHP-FPM

Ensure PHP-FPM is running and listening on a socket:

```bash
# Check if PHP-FPM is running
systemctl status php8.3-fpm

# Find the socket path
php-fpm8.3 -i | grep "listen ="
# Usually: /run/php/php8.3-fpm.sock
```

If PHP-FPM is not installed:

```bash
# Debian/Ubuntu
sudo apt install php8.3-fpm

# Start and enable
sudo systemctl start php8.3-fpm
sudo systemctl enable php8.3-fpm
```

## Step 3: Create Configuration

Create `gateway.yaml` in your project root:

```yaml
schema: gateway/v1

server:
  addr: ":8080"
  read_timeout: 30s
  write_timeout: 60s

php:
  binary: /usr/sbin/php-fpm
  max_children: 20
  start_servers: 2
  request_timeout: 60s

routes:
  - path_prefix: /
    target: /index.php

security:
  protected_patterns:
    - .env
    - .git
    - "*.sql"
```

## Step 4: Run

```bash
# Serve your application
./gateway serve . --php-fpm /run/php/php8.3-fpm.sock

# Or with the config file
./gateway serve . --config gateway.yaml
```

Visit `http://localhost:8080` in your browser.

## Step 5: Validate

```bash
# Check configuration
./gateway config validate --config gateway.yaml

# Check system health
./gateway doctor

# View status
./gateway status
```

## Framework-Specific Setup

### Laravel

```yaml
routes:
  - path_prefix: /public/
    target: /index.php
    strip_prefix: true
  - path_prefix: /
    target: /public/index.php
```

```bash
./gateway serve . --php-fpm /run/php/php8.3-fpm.sock
```

### WordPress

```yaml
routes:
  - path_prefix: /wp-admin/
    target: /wp-admin/index.php
  - path_prefix: /
    target: /index.php
```

### Symfony

```yaml
routes:
  - path_prefix: /
    target: /public/index.php
```

## Runtime Management

```bash
# List installed PHP runtimes
./gateway runtime list

# Install a new version
./gateway runtime install 8.3

# Switch to it
./gateway runtime use 8.3
```

## Development Mode

For local development, enable auto-reload:

```yaml
server:
  addr: "127.0.0.1:8080"
```

```bash
./gateway serve . --php-fpm /run/php/php8.3-fpm.sock
```

The gateway will serve on localhost only and show detailed error pages.

## Next Steps

- [Deployment Guide](DEPLOYMENT.md) — Production deployment
- [Configuration Reference](../gateway.yaml) — Full schema
- [Architecture](../AGENTS.md) — Project structure
