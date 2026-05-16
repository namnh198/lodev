# LODEV

LODEV is a local web development environment manager for containerized projects. It creates and manages project-specific Docker Compose stacks, configures local hostnames and TLS, and provides a single CLI for starting, stopping, inspecting, and extending your development environment.

## Features

- Manage local projects with one command line tool.
- Start, stop, restart, and power off projects and shared services.
- Use Composer inside the project web container.
- Open an interactive shell in the web container with `ssh`.
- Add custom commands from project or global command folders.
- Support for common project types such as PHP, Magento, Laravel, Symfony, Drupal, WordPress, Django, FastAPI, and Node.js.

## Requirements

- Docker Engine
- Docker Compose
- Docker Buildx
- mkcert for local HTTPS support
- Write permissions to modify the system hosts file for automatic hostname configuration
  - MacOS & Linux: `sudo chown $(whoami) /etc/hosts`
  - Windows: `C:\Windows\System32\drivers\etc\hosts`

## Installation

### Homebrew

```bash
brew tap namnh198/homebrew-lodev
brew install namnh198/lodev/lodev
```

### Release builds

Download the latest release archive for your platform from the releases page, extract it, and move the `lodev` binary to a directory in your PATH.

Because LODEV binarys are not signed, you may need to allow it to run on your system:

```bash
# macOS
xattr -d com.apple.quarantine /path/to/lodev
# Linux
chmod +x /path/to/lodev
# Windows
# No additional steps needed, but you may want to unblock the file in the properties dialog.
```

### Build from source

If you build from source, you also need Go 1.26.2 or newer.

```bash
git clone https://github.com/namnh198/lodev.git
cd lodev
go build -o lodev .
```

## Quick Start

1. Create or open a project directory.
2. Initialize the project with LODEV.
3. Start the environment.

```bash
lodev create
lodev start
```

If you want to name the project or choose a project type explicitly:

```bash
lodev create my-project --type=magento
```

## Common Commands

```bash
lodev start [projectname]
lodev stop [project-name]
lodev restart [projectname]
lodev show
lodev logs
lodev ssh
lodev composer <command>
lodev service
lodev service start
lodev service stop
lodev service restart
lodev poweroff
lodev upgrade
```

### Command notes

- `lodev start` starts the active project in the current directory when no project name is provided.
- `lodev stop` can delete the project data with `--delete`.
- `lodev ssh` opens a shell in the web container by default.
- `lodev composer` runs Composer inside the web container and starts the project automatically if needed.
- `lodev service` manages shared services and can list, add, remove, start, stop, or refresh the service registry.

## Project Types

LODEV can create projects using these types:

- `php`
- `magento`
- `openmage`
- `laravel`
- `symfony`
- `drupal`
- `wordpress`
- `python`
- `django`
- `fastapi`
- `nodejs`

If no type is specified, LODEV falls back to generic PHP.

## Configuration

LODEV stores global state in `~/.lodev/` and project state in the project directory.

- Global config: `~/.lodev/global_config.yaml`
- Project registry: `~/.lodev/project_list.yaml`
- Project config: `<project>/.lodev/config.yaml`

You can edit the project config directly after creation if you need to adjust PHP, webserver, ports, extra hosts, environment variables, or connected services.

## Custom Commands

LODEV loads executable custom commands from these directories:

- `<project>/.lodev/commands`
- `~/.lodev/commands`

Custom commands are grouped by service directory name. Host commands live in a `host` directory, while container commands are executed inside the matching service container.

## License

See the repository for licensing details and sample project assets.
