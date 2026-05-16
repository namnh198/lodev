# lodev-webserver Docker Image

A production-ready web server container for PHP applications, built on top of lodev-php, featuring Nginx, Apache2, and PHP-FPM with comprehensive web server configuration tools.

## Summary

The lodev-webserver image extends **lodev-php** with web server capabilities (Nginx and Apache2) and is a critical component of the [lodev](https://github.com/namnh198/lodev) local development environment. It provides a complete PHP web application stack ready for development workflows with support for Magento, WordPress, Laravel, Symfony, Drupal, and other PHP frameworks.

**Key characteristics:**
- Built on top of lodev-php for complete PHP ecosystem
- Dual web server support (Nginx and Apache2)
- PHP-FPM process manager with multiple version support
- Local HTTPS certificate generation (mkcert)
- Framework-specific CLI tools (Magerun, WP-CLI, Drush)
- Supervisor for process management
- Git, rsync, and development utilities
- Arbitrary user support with privileged port binding

## Included Components

### Web Servers
- **Nginx** - Modern, lightweight web server with CAP_NET_BIND_SERVICE for non-root port binding
- **Apache2** - Full-featured web server with modules support

### PHP Environment
- All PHP versions from lodev-php (5.6 - 8.5)
- PHP-FPM with runtime version switching
- Dynamic alternatives configuration for easy version switching

### Development Tools
- **mkcert** - Local CA and certificate generator for HTTPS
- **Git** - Version control
- **rsync** - File synchronization
- **MySQL Client** - MySQL/MariaDB CLI tools
- **PostgreSQL Client** - PostgreSQL CLI tools
- **SQLite3** - SQLite database support
- **Drush** - Drupal CLI (with backdrop extension)
- **Supervisor** - Process manager for services

### Utilities
- **Bash Completion** - Enhanced shell completion support
- **Cron** - Task scheduling
- **gettext** - Internationalization support
- **GraphViz** - Graph visualization
- **jq** - JSON query tool
- **less** - Text pager
- **Python 3** - Python runtime
- **Unzip** - Archive extraction
- **Vim** (tiny version) - Text editor
- **pv** - Progress viewer for pipes
- **patch** - Patch utility
- **ImageMagick** - with HEIF support plugins

### Framework-Specific Tools (via lodev-php)
- **Composer** - PHP dependency manager
- **Symfony CLI** - Symfony framework tools
- **WP-CLI** - WordPress CLI
- **Magerun / Magerun2** - Magento CLI tools
- **n** - Node.js version manager
- **npm, yarn, gulp** - JavaScript tools

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NGINX_SITE_TEMPLATE` | `/etc/nginx/nginx-site.conf` | Nginx site configuration template path |
| `APACHE_SITE_TEMPLATE` | `/etc/apache2/apache-site.conf` | Apache site configuration template path |
| `TERMINUS_CACHE_DIR` | `/mnt/lodev_default/data/terminus/cache` | Terminus (Pantheon) cache directory |
| `CAROOT` | `/mnt/lodev_default/data/mkcert` | Certificate root directory for mkcert |
| All environment variables from lodev-php | See lodev-php README | Inherited PHP environment variables |

## Capabilities

The container is configured with `CAP_NET_BIND_SERVICE` capability for both Nginx and Apache2, allowing non-root users to bind to privileged ports (< 1024).

## Supported Architectures

- `linux/amd64` - x86-64 architecture
- `linux/arm64` - ARM64 architecture (Apple Silicon, etc.)

## How to Use

### Using with lodev

The lodev-webserver image is automatically used by the lodev CLI for all PHP-based projects. Create and start a project:

```bash
lodev create --type php
lodev start
```

The lodev framework manages web server configuration, volume mounts, networking, and certificate generation.

### Direct Docker Usage

For standalone use:

```bash
docker run -d \
  --name web-app \
  -v /path/to/project:/app \
  -p 80:80 \
  -p 443:443 \
  -p 9000:8080 \
  namnh198/lodev-webserver:latest
```

### Web Server Configuration

**Nginx** and **Apache2** can be configured through:
- `$NGINX_SITE_TEMPLATE` - Nginx site configuration
- `$APACHE_SITE_TEMPLATE` - Apache site configuration

Switch between web servers or enable both as needed in your Docker Compose configuration.

### Local HTTPS Setup

Generate local certificates with mkcert:

```bash
mkcert example.test
# Generates: example.test.pem and example.test-key.pem
```

Certificates are stored in `$CAROOT` (typically `/mnt/lodev_default/data/mkcert`).

### Web Server File Structure

Key directories:
- `/etc/nginx/sites-enabled/` - Enabled Nginx sites
- `/var/log/nginx/` - Nginx logs
- `/var/log/apache2/` - Apache2 logs
- `/var/lib/apache2/module/enabled_by_admin/` - Admin-enabled Apache modules
- `/var/lib/apache2/module/disabled_by_admin/` - Admin-disabled Apache modules

## Building

To build the image:

```bash
make images
```

To run tests after building:

```bash
make test
```

To set a custom version:

```bash
make images VERSION=1.2.3
```

### Build Output

The build creates a Docker image with the tag: `{DOCKER_ORG}/lodev-webserver:{VERSION}`

## Base Image

Built on top of lodev-php, which is based on `debian:trixie-slim`, ensuring a complete and minimal Linux environment optimized for web application development.

## License

This image and its source code are licensed under the **Apache License 2.0**. See [LICENSE](../../LICENSE) for details.

## Contributing

Contributions are welcome! Please submit issues and pull requests on the [lodev GitHub repository](https://github.com/namnh198/lodev).
