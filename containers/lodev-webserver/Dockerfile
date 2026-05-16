### ---------------------------base--------------------------------
### Build lodev-php from lodev-webserver-base
ARG BASEIMAGE=namnh198/lodev-php:v0.1.0
FROM ${BASEIMAGE} AS base

ENV DEBIAN_FRONTEND=noninteractive
ENV NGINX_SITE_TEMPLATE=/etc/nginx/nginx-site.conf
ENV APACHE_SITE_TEMPLATE=/etc/apache2/apache-site.conf
ENV TERMINUS_CACHE_DIR=/mnt/lodev_default/data/terminus/cache
ENV CAROOT=/mnt/lodev_default/data/mkcert

ARG TARGETPLATFORM
ARG TARGETARCH
ARG BUILDPLATFORM

SHELL ["/bin/bash", "-eu", "-o", "pipefail", "-c"]

ADD webserver-skel /

RUN /sbin/mkhomedir_helper www-data

RUN DEBIAN_FRONTEND=noninteractive apt-get -qq update -y && apt-get -qq install -y --no-install-recommends --no-install-suggests -y \
  bash-completion \
  cron \
  gettext \
  git \
  gpgv \
  graphviz \
  jq \
  less \
  libcap2-bin \
  imagemagick \
  apache2 \
  nginx \
  locales \
  patch \
  pv \
  python-is-python3 \
  rsync \
  sqlite3 \
  mariadb-client \
  supervisor \
  unzip \
  vim-tiny \
  xz-utils \
  zip

RUN update-alternatives --install /usr/bin/vim vim /usr/bin/vim.tiny 100 && update-alternatives --set vim /usr/bin/vim.tiny

# Arbitrary user needs to be able to bind to privileged ports (for nginx and apache2)
RUN setcap CAP_NET_BIND_SERVICE=+eip /usr/sbin/nginx
RUN setcap CAP_NET_BIND_SERVICE=+eip /usr/sbin/apache2

RUN mkdir -p /etc/nginx/sites-enabled /var/lock/apache2 /var/log/apache2 /var/run/apache2 /var/lib/apache2/module/enabled_by_admin /var/lib/apache2/module/disabled_by_admin && \
  chmod ugo+rw /var/log/php-fpm.log && \
  chmod ugo+rwx /var/run && \
  touch /var/log/nginx/access.log && \
  touch /var/log/nginx/error.log && \
  chmod -R ugo+rw /var/log/nginx/ && \
  chmod ugo+rx /usr/local/bin/*

RUN apt-get -qq autoremove && apt-get -qq clean -y && rm -rf /var/lib/apt/lists/* /tmp/*

# Register php-fpm as an alternative so versions can be switched at runtime
RUN for v in /usr/sbin/php-fpm[0-9]*; do \
  ver="${v#/usr/sbin/php-fpm}"; \
  prio="${ver//./}"; \
  update-alternatives --install /usr/sbin/php-fpm php-fpm "$v" "$prio"; \
  done

RUN <<EOF
  set -eu -o pipefail
  gh_tag() { curl -L --fail --silent "https://api.github.com/repos/$1/releases/latest" | jq -r .tag_name; }

  # Install mkcert
  tag=$(gh_tag FiloSottile/mkcert)
  curl --fail -sSL "https://github.com/FiloSottile/mkcert/releases/download/${tag}/mkcert-${tag}-linux-${TARGETARCH}" --output /usr/local/bin/mkcert
  chmod +x /usr/local/bin/mkcert

  # Install magerun and magerun2 for Magento
  curl --fail -sSL https://files.magerun.net/n98-magerun-latest.phar -o /usr/local/bin/magerun
  chmod 755 /usr/local/bin/magerun
  curl --fail -sSL https://raw.githubusercontent.com/netz98/n98-magerun/master/res/autocompletion/bash/n98-magerun.phar.bash -o /etc/bash_completion.d/n98-magerun.phar
  curl --fail -sSL https://files.magerun.net/n98-magerun2-latest.phar -o /usr/local/bin/magerun2
  chmod 755 /usr/local/bin/magerun2
  curl --fail -sSL https://raw.githubusercontent.com/netz98/n98-magerun2/master/res/autocompletion/bash/n98-magerun2.phar.bash -o /etc/bash_completion.d/n98-magerun2.phar

  # Install backdrop drush extension
  tag=$(gh_tag backdrop-contrib/drush)
  curl --fail -sSL "https://github.com/backdrop-contrib/drush/releases/download/${tag}/backdrop-drush-extension.zip" -o /tmp/backdrop-drush-extension.zip
  unzip -o /tmp/backdrop-drush-extension.zip -d /var/tmp/backdrop_drush_commands
  chmod -R ugo+w /var/tmp/backdrop_drush_commands
  rm -f /tmp/backdrop-drush-extension.zip
EOF

# Set up Blackfire apt repository
RUN curl -fSsL https://packages.blackfire.io/gpg.key | gpg --dearmor -o /usr/share/keyrings/blackfire-archive-keyring.gpg && \
  echo "deb [signed-by=/usr/share/keyrings/blackfire-archive-keyring.gpg] http://packages.blackfire.io/debian any main" | tee /etc/apt/sources.list.d/blackfire.list
### -------------------------END base------------------------------

### -------------------------lodev-webserver-base------------------------------
### Build lodev-webserver-dev-base from lodev-webserver-base
FROM base AS lodev-webserver-base
SHELL ["/bin/bash", "-eu", "-o", "pipefail", "-c"]

ENV PHP_DEFAULT_VERSION="8.4"

RUN apt-get -qq update && apt-get -qq install --no-install-recommends --no-install-suggests -y \
  blackfire \
  blackfire-php \
  fontconfig \
  iproute2 \
  iputils-ping \
  libpcre3 \
  nano \
  ncurses-bin \
  netcat-traditional \
  sudo \
  telnet \
  tree

# ping needs more privileges on lima-rootless
RUN setcap cap_net_raw+p /usr/bin/ping

# blackfire user by default is set up with /dev/null as homedir, and 999 as uid, which
# can break people. Use a real homedir
RUN mkdir -p /home/blackfire && chown blackfire:blackfire /home/blackfire && usermod -d /home/blackfire blackfire

COPY webserver-base-files /
RUN phpdismod blackfire xdebug

RUN <<EOF
  set -eu -o pipefail
  gh_tag() { curl -L --fail --silent "https://api.github.com/repos/$1/releases/latest" | jq -r .tag_name; }

  # Install terminus cli
  tag=$(gh_tag pantheon-systems/terminus)
  curl --fail -sSL "https://github.com/pantheon-systems/terminus/releases/download/${tag}/terminus.phar" --output /usr/local/bin/terminus
  chmod 755 /usr/local/bin/terminus

  # Install upsun cli
  curl -fsSL https://raw.githubusercontent.com/platformsh/cli/main/installer.sh | VENDOR=upsun bash
  #apt modernize-sources --assume-yes
  rm -f /etc/apt/sources.list.d/*.list.bak
EOF

RUN mkdir -p /etc/nginx/sites-enabled /var/log/apache2 /var/run/apache2 /var/lib/apache2/module/enabled_by_admin /var/lib/apache2/module/disabled_by_admin /opt/phpstorm-coverage && \
  chmod a+rw /opt/phpstorm-coverage && \
  touch /var/log/php-fpm.log && \
  chmod ugo+rw /var/log/php-fpm.log && \
  chmod ugo+rwx /var/run && \
  touch /var/log/nginx/access.log && \
  touch /var/log/nginx/error.log && \
  chmod -R ugo+rw /var/log/nginx/ && \
  chmod ugo+rwx /usr/local/bin/* && \
  update-alternatives --set php-fpm /usr/sbin/php-fpm${PHP_DEFAULT_VERSION}

RUN chmod -R 777 /var/log

# we need to create the /var/cache/nginx and /var/lib/nginx manually for the arm64 image and chmod them, please don't remove them!
RUN mkdir -p /mnt/lodev_default/traefik/mkcert /run/{php,blackfire} /var/cache/nginx /var/lib/nginx && chmod -R ugo+rw /mnt/lodev_default/

RUN chmod -fR ugo+w /etc/nginx /var/cache/nginx /var/lib/nginx /run /var/www /etc/php/*/*/conf.d/ /var/lib/php/modules /etc/alternatives /var/lib/dpkg/alternatives /usr/local/lib/node_modules /etc/php /etc/apache2 /var/log/apache2/ /var/run/apache2 /var/lib/apache2 /mnt/lodev_default/*

RUN touch /var/log/nginx/error.log /var/log/nginx/access.log /var/log/php-fpm.log && \
  chmod 666 /var/log/nginx/error.log /var/log/nginx/access.log /var/log/php-fpm.log

RUN a2dismod mpm_event && a2enmod ssl headers expires

RUN chmod ugo+x /start.sh /healthcheck.sh

# Composer, etc may need to be updated by composer self-update
RUN chmod -f ugo+rwx /usr/local/bin /usr/local/bin/*

RUN chmod ugo+w /etc/ssl/certs /usr/local/share/ca-certificates

HEALTHCHECK --interval=1s --retries=120 --timeout=120s --start-period=120s CMD ["/healthcheck.sh"]
CMD ["/start.sh"]
RUN apt-get -qq autoremove -y && apt-get -qq clean -y && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/share/doc/* /usr/share/man/*
RUN update-alternatives --set php /usr/bin/php${PHP_DEFAULT_VERSION}
### -------------------------END webserver-base--------------------------

### -----------------------------lodev-webserver-----------------------------------
### This could be known as lodev-webserver as it's development-env targeted
### But for historical reasons, it's just lodev-webserver
### Build lodev-webserver by turning lodev-webserver-base into one layer
FROM lodev-webserver-base AS lodev-webserver
ENV PHP_DEFAULT_VERSION="8.4"
ENV NGINX_SITE_TEMPLATE=/etc/nginx/nginx-site.conf
ENV APACHE_SITE_TEMPLATE=/etc/apache2/apache-site.conf
ENV TERMINUS_CACHE_DIR=/mnt/lodev_default/data/terminus/cache
ENV TERMINUS_HIDE_UPDATE_MESSAGE=1
ENV CAROOT=/mnt/lodev_default/data/mkcert
ENV COMPOSER_ALLOW_SUPERUSER=1
ENV COMPOSER_CACHE_DIR=/mnt/lodev_default/data/composer
ENV COMPOSER_PROCESS_TIMEOUT=2000
ENV DEBIAN_FRONTEND=noninteractive
ENV TERM=xterm
ENV BASH_ENV=/etc/bash.nointeractive.bashrc
ENV LANG=C.UTF-8
ENV XHPROF_OUTPUT_DIR=/tmp/xhprof
ENV PLATFORMSH_CLI_UPDATES_CHECK=0

COPY --from=lodev-webserver-base / /
HEALTHCHECK --interval=1s --retries=120 --timeout=120s --start-period=120s CMD ["/healthcheck.sh"]
CMD ["/bin/bash", "/pre-start.sh"]
### ----------------------------END lodev-webserver--------------------------------
