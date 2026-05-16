package nodeps

// InstructmentationConfig is used to instruct the user about how to configuration LODEV, provide some useful information about LODEV.
const InstructmentationConfig = `
# version_constraint: ""
# Example:
# version_constraint: ">= 0.1.0"
# That's required LODEV services has min version as 0.1.0, otherwise, LODEV will not start and print an error message

# In unusual cases the default value to wait to detect internet availability is too short.
# You can adjust this value higher to make it less likely that  will declare internet
# unavailable, but LODEV may wait longer on some commands. This should not be set below the default 3000
# LODEV will ignore low values, as they're not useful
# internet_wait_timeout: "3000"

# container_wait_timeout: "120"
# You can adjust this value higher to make it less likely that LODEV will declare a container
# failed to start, but LODEV may wait longer on some commands. This should not be set below the default 120
# LODEV will ignore low values, as they're not useful

# You can set the global project_tld. This way any project will use this tld. If not
# set the local project_tld is used, or the default of .
# project_tld: ""

# router: "traefik"
# The default router to use for projects. Currenly only "traefik" is supported, but in the future we may add support for other routers like nginx-proxy or caddy.

# last_started_version: ""
# The last LODEV version that was started (informational only)

# mkcert_caroot: ""
# Absolute path to the directory containing mkcert certificates (from 'mkcert -CAROOT')

# You can inject environment variables into the web container with:
# web_environment:
#     - MAGEMODE=developer
#     - SOMEENV=somevalue

# Lets Encrypt:
# This integration is entirely experimental; your mileage may vary.
# * Your host must be directly internet-connected.
# * DNS for the hostname must be set to point to the host in question
# * You must have router_bind_all_interfaces: true or else the Let's Encrypt Certbot
#   process will not be able to process the IP address of the host (and nobody will be able to access your site)
# * You will need to add a startup script to start your sites after a host reboot.
# * If using several sites at a single top-level domain, you'll probably want to set
#   project_tld to that top-level domain. Otherwise, you can use additional-hostnames
#
# use_letsencrypt: false
# (Experimental, only useful on an internet-based server)
# Set to true if certificates are to be obtained via Certbot on https://letsencrypt.org/
#
# letsencrypt_email: <email>
# Email to be used for experimental letsencrypt certificates

# router_http_port: "80"
# Router port used for HTTP, can be overridden in project config

# router_https_port: "443"
# Router port used for HTTPS, can be overridden in project config

# traefik_monitor_port: "10999"
# Change this only if you're having conflicts with some
# service that needs port 10999

# connected_services: []
# List of services that should be started when the project starts. Services must be defined in the shared_services section of the global config

# use_docker_compose_from_path: false
# This should only be used in specific cases like troubleshooting.
# Whether to use the system-installed docker-compose instead of downloading a specific version

# use_docker_buildx_system: false
# This should only be used in specific cases like troubleshooting.
# Whether to use the system-installed docker-buildx instead of downloading a specific version
`

// InstructmentationProject is used to instruct the user about how to create and manage LODEV projects.
const InstructmentationProject = `
# Key features of LODEV's config.yaml:

# name: <projectname> # Name of the project, automatically provides
#   http://projectname.test
# If the name is omitted, the project will take the name of the enclosing directory,
# which is useful if you want to have a copy of the project side by side with this one.

# type: <projecttype>  # Project type, like "php", "magento", "openmage", "drupal", "wordpress", "django", "fastapi", or "nodejs"

# docroot: <relative_path> # Relative path to the directory containing index.php.

# php_version: "8.4"  # PHP version to use, "5.6" through "8.5"

# You can explicitly specify the webimage but this
# is not recommended, as the images are often closely tied to LODEV's' behavior,
# so this can break upgrades.

# webimage: <docker_image>
# It's unusual to change this option, and we don't recommend it without Docker experience and a good reason.
# Typically, this means additions to the existing web image using a .lodev/web-build/Dockerfile.*

# xdebug_enabled: false  # Set to true to enable Xdebug and "lodev start" or "lodev restart"
# Note that for most people the commands
# "lodev xdebug" to enable Xdebug and "lodev xdebug off" to disable it work better,
# as leaving Xdebug enabled all the time is a big performance hit.

# webserver_type: nginx-fpm, apache-fpm
# The webserver to use, either "nginx-fpm" or "apache-fpm". This is used to determine which webserver configuration template to use. It can be set globally in the global config, but is more commonly set per-project.

# restart_always: false
# Whether to always restart the project containers when they exit. This is useful for projects that have a single long-running process, like a Node.js app, and don't use the webserver for anything other than proxying.

# timezone: Asia/Ho_Chi_Minh
# If timezone is unset, LODEV will attempt to derive it from the host system timezone
# using the $TZ environment variable or the /etc/localtime symlink.
# This is the timezone used in the containers and by PHP;
# it can be set to any valid timezone,
# see https://en.wikipedia.org/wiki/List_of_tz_database_time_zones
# For example Europe/Dublin or MST7MDT

# composer_root: <relative_path>
# Relative path to the Composer root directory from the project root. This is
# the directory which contains the composer.json and where all Composer related
# commands are executed.

# composer_version: "2"
# You can set it to "" or "2" (default) for Composer v2
# to use the latest major version available at the time your container is built.
# It is also possible to use each other Composer version channel. This includes:
#   - 2.2 (latest Composer LTS version)
#   - stable
#   - preview
#   - snapshot
# Alternatively, an explicit Composer version may be specified, for example "2.2.18".

# nodejs_version: "24"
# change from the default system Node.js version to any other version.

# additional_hosts:
#  - somename
#  - someothername
# would provide http and https URLs for "somename.test"
# and "someothername.test".

# working_dir: /var/www/html
# would set the default working directory for the web and db services.
# These values specify the destination directory for  ssh and the
# directory in which commands passed into  exec are run.

# host_https_port: "59002"
# The host port binding for https can be explicitly specified. It is
# dynamic unless otherwise specified.
# This is not used by most people, most people use the *router* instead
# of the localhost port.

# host_webserver_port: "59001"
# The host port binding for the -webserver can be explicitly specified. It is
# dynamic unless otherwise specified.
# This is not used by most people, most people use the *router* instead
# of the localhost port.

# You can inject environment variables into the web container with:
# web_environment:
#     - SOMEENV=somevalue
#     - SOMEOTHERENV=someothervalue

#web_extra_exposed_ports:
#- name: nodejs
#  container_port: 3000
#  http_port: 2999
#  https_port: 3000
#- name: something
#  container_port: 4000
#  https_port: 4000
#  http_port: 3999
# Allows a set of extra ports to be exposed via -router
# Fill in all three fields even if you don't intend to use the https_port!
# If you don't add https_port, then it defaults to 0 and lodev-router will fail to start.

#web_extra_daemons:
#- name: "http-1"
#  command: "/var/www/html/node_modules/.bin/http-server -p 3000"
#  directory: /var/www/html
#- name: "http-2"
#  command: "/var/www/html/node_modules/.bin/http-server /var/www/html/sub -p 3000"
#  directory: /var/www/html
`
