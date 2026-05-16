# lodev-php Docker Image

A comprehensive PHP development container for the lodev project, supporting multiple PHP versions and a rich set of extensions and development tools.

## Summary

The lodev-php image serves as a **base image for the lodev-webserver** and is a core component of the [lodev](https://github.com/namnh198/lodev) local development environment manager. It provides a complete PHP-FPM stack with support for multiple PHP versions, making it suitable for a wide range of PHP-based projects from legacy applications to modern frameworks.

**Key characteristics:**

- Multi-PHP version support (5.6 - 8.5) with runtime switching
- Rich set of pre-installed extensions for common use cases
- Development tools for PHP, Node.js, and popular frameworks
- Optimized for use in Docker Compose-based development environments
- Supports both amd64 and arm64 (Apple Silicon) architectures

## Supported PHP Versions

The image supports the following PHP versions:

- **PHP 5.6** - Legacy support
- **PHP 7.0** - Legacy support
- **PHP 7.1** - Legacy support
- **PHP 7.2** - Legacy support
- **PHP 7.3** - Legacy support
- **PHP 7.4** - Maintained
- **PHP 8.0** - Supported
- **PHP 8.1** - Supported
- **PHP 8.2** - Supported
- **PHP 8.3** - Supported
- **PHP 8.4** - Latest stable (default)
- **PHP 8.5** - Development

## Included PHP Extensions

All supported PHP versions include the following extensions:

- **Core**: cli, common, fpm, json, readline
- **Database**: mysql, pgsql, sqlite3
- **Caching**: apcu, opcache, memcached, redis
- **Compression**: bz2, zip
- **Encoding/Compression**: curl, xml, xmlrpc
- **Image Processing**: gd, imagick
- **Internationalization**: intl, mbstring
- **Utilities**: bcmath, ldap, soap, yaml
- **Development**: xdebug, uploadprogress

> **Note**: PHP 5.6, 7.0, and 7.1 include `mcrypt`. PHP 7.0+ include `apcu-bc` for backward compatibility.

## Included Development Tools

### Package Managers & Build Tools

- **Composer** - PHP dependency manager (latest stable version)
- **npm** - Node.js package manager
- **yarn** - JavaScript package manager
- **gulp-cli** - Task runner for JavaScript

### Framework Tools

- **Symfony CLI** - Symfony framework development tool
- **WP-CLI** - WordPress command-line interface
- **Magerun** - OpenMage CLI utility with bash completion
- **Magerun2** - Magento 2 CLI utility with bash completion

### Node.js

- **Node.js 24** - JavaScript runtime (LTS version)
- **n** - Node.js version manager for easy version switching

### Utilities

- **yq** - YAML/JSON query and processing tool
- **xdebugctl** - CLI tool for managing Xdebug state
- **jq** - JSON query tool
- **ghostscript** - PostScript/PDF interpreter
- **ImageMagick** - Image manipulation library
- **GraphicsMagick** - Graphics manipulation utility
- **msmtp** - SMTP client for mail sending
- **SQLite3** - Embedded database support
- **lsof** - File descriptor inspection

## Environment Variables

| Variable                   | Default                    | Description                         |
| -------------------------- | -------------------------- | ----------------------------------- |
| `LODEV_PHP_VERSION`        | `8.4`                      | Active PHP version in the container |
| `NODE_VERSION`             | `24`                       | Node.js version                     |
| `PHP_INI`                  | `/etc/php/8.4/fpm/php.ini` | PHP configuration file path         |
| `COMPOSE_ALLOW_SUPERUSER`  | `1`                        | Allow Composer to run as root       |
| `COMPOSER_PROCESS_TIMEOUT` | `2000`                     | Composer process timeout in seconds |
| `XDEBUG_MODE`              | `off`                      | Xdebug mode (off by default)        |

## Exposed Ports

- **8080** - PHP-FPM FastCGI port
- **8585** - PHP development server port

## Supported Architectures

- `linux/amd64` - x86-64 architecture
- `linux/arm64` - ARM64 architecture (Apple Silicon, etc.)

## How to Use

### Using with lodev

The lodev-php image is automatically managed through the lodev CLI. When you create a project with lodev:

```bash
lodev create --type php
lodev start
```

The lodev framework handles Docker Compose configuration, volume mounting, and network setup automatically.

### Direct Docker Usage

For standalone use without lodev:

```bash
docker run -d --name php-dev \
  -v /path/to/project:/var/www/html \
  -p 9000:8080 \
  namnh198/lodev-php:latest
```

### PHP Version Management

Switch PHP versions at runtime inside the container:

```bash
# List available PHP versions
ls /usr/bin/php*

# Switch active PHP version
update-alternatives --set php /usr/bin/php8.1

# Verify current version
php --version
```

## Building

To build the image:

```bash
make images
```

To build for specific architectures:

```bash
make images BUILD_ARCHS=linux/amd64,linux/arm64
```

To set a custom version:

```bash
make images VERSION=1.2.3
```

### Multi-architecture Build

The build uses Docker BuildKit for efficient layer caching and is labeled with the build host information.

## Xdebug Management

Xdebug is installed but disabled by default. To enable/disable Xdebug:

```bash
# Enable Xdebug
phpenmod xdebug

# Disable Xdebug
phpdismod xdebug

# Use xdebugctl for command-line control
xdebugctl enable
xdebugctl disable
```

## PHP Configuration

The active PHP-FPM configuration file is located at:

```
/etc/php/{LODEV_PHP_VERSION}/fpm/php.ini
```

The PHP-FPM socket is available at:

```
/run/php/php-fpm.sock
```

## Base Image

This image is built from `debian:trixie-slim`, providing a minimal but complete Linux environment with all necessary dependencies for PHP development.

## License

This image and its source code are licensed under the **Apache License 2.0**. See [LICENSE](../../LICENSE) for details.

## Contributing

Contributions are welcome! Please submit issues and pull requests on the [lodev GitHub repository](https://github.com/namnh198/lodev).
