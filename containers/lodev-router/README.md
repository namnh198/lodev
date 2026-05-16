# lodev-router Docker Image

A Traefik-based reverse proxy container designed for intelligent request routing, SSL/TLS termination, and service discovery in the lodev development environment.

## Summary

The lodev-router image provides a **Traefik v3.6 reverse proxy** that serves as the central routing component of the [lodev](https://github.com/namnh198/lodev) local development environment. It handles hostname-based routing, dynamic service discovery, SSL/TLS termination, and request monitoring for all containerized projects.

**Key characteristics:**
- Traefik v3.6 - Modern, dynamic reverse proxy
- Automatic service discovery
- Hostname-based routing for local development
- Health checks with liveness probes
- Request monitoring and debugging
- Ultra-lightweight Alpine Linux base
- Supports both amd64 and arm64 architectures

## Included Components

### Core Service
- **Traefik v3.6** - Reverse proxy and load balancer
- **Alpine Linux 3.23** - Minimal, secure base image

### Utilities
- **bash** - Shell scripting
- **curl** - HTTP client for health checks
- **file** - File type identification
- **htop** - Process viewer
- **jq** - JSON query tool
- **openssl** - Cryptography and SSL/TLS tools
- **vim** - Text editor
- **yq** - YAML/JSON query tool

### Monitoring
- **Health check** - Container liveness probe with 120s grace period
- **Traefik stderr monitoring** - Custom monitoring script for Traefik diagnostics

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TRAEFIK_MONITOR_PORT` | `10999` | Port for Traefik monitoring and diagnostics |

## Exposed Ports

- **80** - HTTP traffic
- **443** - HTTPS traffic
- **10999** - Traefik monitoring/diagnostics port

## Supported Architectures

- `linux/amd64` - x86-64 architecture
- `linux/arm64` - ARM64 architecture (Apple Silicon, etc.)

## Configuration

### Default Configuration Location

Traefik configuration file location:
```bash
/etc/traefik/traefik.yaml → /mnt/lodev_default/traefik/.static_config.yaml
```

The container uses a symlink from the standard Traefik configuration location to the lodev working directory for persistent configuration.

### Working Directory

- **Mount Point**: `/mnt/lodev_default/traefik`
- **Contains**: Static configuration, dynamic config, certificates, and logs

## How to Use

### Using with lodev

The lodev-router image is automatically managed by the lodev CLI as the central routing service:

```bash
lodev create
lodev start
```

The router handles all hostname-based routing across your projects automatically.

### Direct Docker Usage

For standalone use with custom Traefik configuration:

```bash
docker run -d \
  --name traefik-router \
  -v /path/to/traefik/config:/mnt/lodev_default/traefik \
  -p 80:80 \
  -p 443:443 \
  -p 10999:10999 \
  namnh198/lodev-router:latest
```

### Health Checks

The container includes a health check that:
- Checks every 1 second
- Times out after 120 seconds
- Uses 1 retry
- Has a 120s startup grace period

Health check script location: `/healthcheck.sh`

### Monitoring

Monitor Traefik diagnostics and routing information:

```bash
# Access monitoring on port 10999
curl http://localhost:10999/
```

### Stderr Monitoring

The container runs a custom monitoring script (`monitor-traefik-stderr.sh`) that:
- Captures Traefik stderr output
- Logs routing events and diagnostics
- Helps troubleshoot request routing issues

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

The build creates a multi-architecture Docker image with the tag: `{DOCKER_ORG}/lodev-router:{VERSION}`

## Base Image

Built on **Alpine Linux 3.23**, providing a minimal, secure, and efficient container environment optimized for lightweight network services.

## License

This image and its source code are licensed under the **Apache License 2.0**. See [LICENSE](../../LICENSE) for details.

## Contributing

Contributions are welcome! Please submit issues and pull requests on the [lodev GitHub repository](https://github.com/namnh198/lodev).
