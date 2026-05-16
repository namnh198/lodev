# LODEV Setup Guide

This document walks through the step-by-step setup for LODEV, from installing the binary to creating and running your first project.

## 1. Prerequisites

Before you install LODEV, make sure these tools are available on your machine:

- Docker Engine
- Docker Compose
- Docker Buildx
- mkcert for local HTTPS certificates
- Go 1.26.2 or newer if you plan to build from source

On macOS, Docker Desktop typically provides Docker Engine, Docker Compose, and Buildx. If you do not already have mkcert installed, install it before running LODEV so local TLS can be configured properly.

## 2. Get the Source Code

Clone the repository and move into the project directory:

```bash
git clone https://github.com/namnh198/lodev.git
cd lodev
```

## 3. Build the Binary

Build LODEV from source with Go:

```bash
go build -o lodev .
```

If you prefer to install it into your Go bin directory, you can also use:

```bash
go install .
```

## 4. Verify the Installation

Run the binary to confirm it starts correctly and shows the CLI help:

```bash
./lodev
```

If you install it into your PATH, you can run:

```bash
lodev
```

The first run will create the global LODEV configuration directory in `~/.lodev/` when it does not already exist.

## 5. Create a Project

Change into the application directory that you want LODEV to manage, then create the project configuration:

```bash
cd /path/to/your/project
lodev create
```

You can also provide a project name and type explicitly:

```bash
lodev create my-project --type=magento
```

Supported project types include:

- php
- magento
- openmage
- laravel
- symfony
- drupal
- wordpress
- python
- django
- fastapi
- nodejs

If no project type is provided, LODEV falls back to generic PHP.

## 6. Review the Project Configuration

After creation, LODEV stores the project config in:

```text
<project>/.lodev/config.yaml
```

Use this file to adjust project details such as:

- project name
- project type
- docroot
- PHP version
- webserver settings
- additional hosts
- environment variables
- connected services

## 7. Start the Environment

Start the project from the project directory:

```bash
lodev start
```

If you are outside the project directory, you can start a named project instead:

```bash
lodev start my-project
```

When startup completes, the project is typically available at the primary hostname shown by LODEV.

## 8. Manage the Environment

These commands are the main day-to-day controls:

```bash
lodev show
lodev logs
lodev ssh
lodev restart
lodev stop
lodev poweroff
```

Use them as follows:

- `lodev show` displays configuration and project information.
- `lodev logs` streams the running project logs.
- `lodev ssh` opens a shell in the web container.
- `lodev restart` restarts the project.
- `lodev stop` stops the project.
- `lodev poweroff` stops the full LODEV environment, including shared services and router components.

## 9. Manage Shared Services

Shared services are controlled with the `service` command group:

```bash
lodev service
lodev service start
lodev service stop
lodev service restart
lodev service --update
lodev service --add mysql,phpmyadmin,mailpit
lodev service --remove mailpit
```

Use `lodev service` without flags to list available services and their location.

## 10. Use Composer Inside the Container

Run Composer in the project web container with:

```bash
lodev composer install
```

This is the recommended way to run Composer so dependencies are installed in the same container context used by the project.

## 11. Add Custom Commands

You can extend LODEV with executable custom commands stored in either of these folders:

- `<project>/.lodev/commands`
- `~/.lodev/commands`

Custom commands are grouped by service directory name. A `host` directory is used for host-side commands, while other service directories run inside matching containers.

## 12. Configuration Files

LODEV keeps its persistent state in the following locations:

- Global config: `~/.lodev/global_config.yaml`
- Project registry: `~/.lodev/project_list.yaml`
- Project config: `<project>/.lodev/config.yaml`

If something does not behave as expected, these are the first files to check.

## 13. Update LODEV

When a new version is available, update using the CLI or rebuild from source, depending on how you installed it.

```bash
lodev upgrade
```

For release automation in this repository, see `release_version.sh`.

## 14. Troubleshooting

- If LODEV reports a Docker version problem, verify Docker Engine, Docker Compose, and Buildx are installed and available in your shell.
- If HTTPS or certificate generation fails, confirm `mkcert` is installed and that its root CA is readable.
- If a project cannot be found, make sure you are in the correct project directory or pass the project name explicitly.
- If configuration looks stale, check the `~/.lodev/` files and re-run the setup steps for the project.

## Next Step

Once the setup is complete, open the root [README.md](../README.md) for the command overview and project summary.
