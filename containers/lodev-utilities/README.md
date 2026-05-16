# lodev-utilities Docker Image

A lightweight utility container providing essential tools for certificate management and local development infrastructure in the lodev environment.

## Summary

The lodev-utilities image provides **utility services and tools** for the [lodev](https://github.com/namnh198/lodev) local development environment. It serves as a helper container for infrastructure tasks, particularly certificate generation and management with mkcert.

**Key characteristics:**
- Lightweight Alpine Linux base
- Pre-installed mkcert for local CA and SSL/TLS certificate generation
- Essential utilities for scripting and inspection (jq, yq, curl, wget)
- Pre-initialized local certificate authority
- Supports both amd64 and arm64 architectures
- Minimal footprint for background services

## Included Components

### Certificate Management
- **mkcert** - Simple local CA for HTTPS certificate generation

### System Tools
- **bash** - Shell scripting
- **curl** - HTTP client
- **jq** - JSON query tool
- **perl-utils** - Perl utilities
- **wget** - File downloader
- **openssl** - SSL/TLS cryptography tools
- **nss** - Network Security Services

### Base
- **Alpine Linux 3.23** - Minimal Linux distribution

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CAROOT` | `/mnt/lodev_default/traefik/mkcert` | Certificate root directory for mkcert |

## Supported Architectures

- `linux/amd64` - x86-64 architecture
- `linux/arm64` - ARM64 architecture (Apple Silicon, etc.)

## Certificate Management

### Pre-initialized Certificates

The image comes with a pre-installed local certificate authority. Certificates are stored in:
```
/mnt/lodev_default/traefik/mkcert
```

### Generating Local Certificates

Use mkcert to generate certificates for local development:

```bash
docker run --rm \
  -v /path/to/certs:/certs \
  namnh198/lodev-utilities \
  mkcert -cert-file /certs/app.test.pem -key-file /certs/app.test-key.pem app.test
```

### Certificate Authority

The local CA is automatically initialized in the image. To trust it on your system, mount and use the CA certificate:
```
$CAROOT/rootCA.pem
```

## How to Use

### Using with lodev

The lodev-utilities container is typically used internally by lodev for background utility services:

```bash
lodev create
lodev start
# Utilities container runs as needed for certificate management
```

### Direct Docker Usage

For standalone utility operations:

```bash
# Interactive shell
docker run -it \
  -v /path/to/work:/work \
  namnh198/lodev-utilities

# Query JSON
docker run --rm \
  -i namnh198/lodev-utilities \
  jq '.' < data.json

# Download files
docker run --rm \
  -v /path/to/download:/download \
  namnh198/lodev-utilities \
  wget https://example.com/file.txt -O /download/file.txt
```

### Scripting

Use as a base for development scripts or CI/CD pipelines that need JSON/YAML processing:

```bash
docker run --rm \
  -v /path/to/config:/config \
  namnh198/lodev-utilities \
  yq '.services' /config/docker-compose.yml
```

## Building

To build the image:

```bash
make multi-arch
```

To build for specific architectures:

```bash
make multi-arch BUILD_ARCHS=linux/amd64,linux/arm64
```

To set a custom version:

```bash
make multi-arch VERSION=1.2.3
```

### Build Output

The build creates a multi-architecture Docker image with the tag: `{DOCKER_ORG}/lodev-utilities:{VERSION}`

## Base Image

Built on **Alpine Linux 3.23**, providing an extremely lightweight (< 50MB) container with all essential utilities for infrastructure tasks.

## License

This image and its source code are licensed under the **Apache License 2.0**. See [LICENSE](../../LICENSE) for details.

## Contributing

Contributions are welcome! Please submit issues and pull requests on the [lodev GitHub repository](https://github.com/namnh198/lodev).
