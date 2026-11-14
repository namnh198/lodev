FROM debian:trixie-slim

ARG PHP_VERSIONS
ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-eu", "-o", "pipefail", "-c"]

# Base packages and the deb.sury.org PHP repository; PHP itself is installed below.
# apt's lists and downloaded .debs live in BuildKit cache mounts (not in the image): a layer
# that reruns installs from them instead of downloading again.
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    test -n "$PHP_VERSIONS" \
    && rm -f /etc/apt/apt.conf.d/docker-clean \
    && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' > /etc/apt/apt.conf.d/keep-cache \
    && apt-get update --error-on=any \
    && apt-get install -y --no-install-recommends \
       ca-certificates curl nginx apache2 supervisor default-mysql-client git unzip sudo \
       nano vim-tiny sqlite3 jq gcc g++ make autoconf pkg-config redis-tools php-pear \
    && curl -fsSL -o /tmp/sury-keyring.deb https://packages.sury.org/debsuryorg-archive-keyring.deb \
    && echo "7511384559c9ddf1d5ce5f60be429ae9d4e7d01d9480d6f1b7a30c0810cf8b60  /tmp/sury-keyring.deb" | sha256sum -c - \
    && dpkg -i /tmp/sury-keyring.deb \
    && . /etc/os-release \
    && printf 'Types: deb\nURIs: https://packages.sury.org/php/\nSuites: %s\nComponents: main\nSigned-By: /usr/share/keyrings/deb.sury.org-php.gpg\n' \
       "$VERSION_CODENAME" > /etc/apt/sources.list.d/php.sources \
    && update-alternatives --install /usr/bin/vim vim /usr/bin/vim.tiny 10 \
    && rm -f /etc/nginx/sites-enabled/default \
    && a2dismod -f mpm_prefork \
    && a2enmod mpm_event proxy_fcgi proxy_http rewrite headers setenvif \
    && rm -rf /tmp/* /var/log/apt/* /var/log/dpkg.log

COPY install-php.sh /usr/local/bin/lodev-install-php
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked --mount=type=cache,target=/var/lib/apt/lists,sharing=locked lodev-install-php $PHP_VERSIONS

# Every tool is pinned and checked (assets/pins_test.go keeps it so): bump the version and its sha256 together.
COPY --from=composer:2.11.2 /usr/bin/composer /usr/local/bin/composer
# Mailpit's binary doubles as a sendmail replacement for PHP mail().
COPY --from=axllent/mailpit:v1.33.1 /mailpit /usr/local/bin/mailpit
RUN curl -fsSL -o /usr/local/bin/wp https://github.com/wp-cli/wp-cli/releases/download/v2.12.0/wp-cli-2.12.0.phar \
    && echo "ce34ddd838f7351d6759068d09793f26755463b4a4610a5a5c0a97b68220d85c  /usr/local/bin/wp" | sha256sum -c - \
    && curl -fsSL -o /usr/local/bin/n98-magerun2 https://files.magerun.net/n98-magerun2-10.1.0.phar \
    && echo "76c931312f3ab6a5a5e1dbd40544cf43f9ecab176bca6a3597ebf4a3413fe270  /usr/local/bin/n98-magerun2" | sha256sum -c - \
    && chmod +x /usr/local/bin/wp /usr/local/bin/n98-magerun2

# supercronic runs the project's cron jobs (cron: true): as the project user, with the container env.
ARG SUPERCRONIC=v0.2.51
RUN arch=$(dpkg --print-architecture) && case $arch in \
      amd64) sum=cc232108374e8e2b537045f2d5eaa1229f619db314c3ba6a14d38b35dc0aa123 ;; \
      arm64) sum=4ae22576a78223d8ccdf5f3b3660a5cbb5288416bf0ca96afc4a36d195932b94 ;; \
      *) echo "no supercronic for $arch" >&2; exit 1 ;; esac \
    && curl -fsSL -o /usr/local/bin/supercronic \
       "https://github.com/aptible/supercronic/releases/download/$SUPERCRONIC/supercronic-linux-$arch" \
    && echo "$sum  /usr/local/bin/supercronic" | sha256sum -c - \
    && chmod +x /usr/local/bin/supercronic

# mongosh, the MongoDB shell (the plain tarball runs on Debian's glibc and OpenSSL).
ARG MONGOSH=2.14.2
RUN arch=$(dpkg --print-architecture) && case $arch in \
      amd64) mongoarch=x64; sum=1523e16da7c6902333b681bf65d83bbef59bff558c46ac5636371d13beb4de53 ;; \
      arm64) mongoarch=arm64; sum=20f89386f74c14cb63a163eea5491dc14759bcd64229b1c099c2390585a01fd3 ;; \
      *) echo "no mongosh for $arch" >&2; exit 1 ;; esac \
    && curl -fsSL -o /tmp/mongosh.tgz \
       "https://downloads.mongodb.com/compass/mongosh-$MONGOSH-linux-$mongoarch.tgz" \
    && echo "$sum  /tmp/mongosh.tgz" | sha256sum -c - \
    && tar -xzf /tmp/mongosh.tgz -C /tmp "mongosh-$MONGOSH-linux-$mongoarch/bin/mongosh" \
    && install -m 0755 "/tmp/mongosh-$MONGOSH-linux-$mongoarch/bin/mongosh" /usr/local/bin/mongosh \
    && rm -rf /tmp/mongosh*

# Node.js via n, into /usr/local like a project's nodejs: (which reinstalls over it).
# n keeps downloaded versions in /usr/local/n: a cache mount, so a rebuild skips the download.
ARG NODE_VERSION=24
RUN --mount=type=cache,target=/usr/local/n,sharing=locked \
    curl -fsSL -o /usr/local/bin/n https://raw.githubusercontent.com/tj/n/v10.1.0/bin/n \
    && echo "a09599719bd38af5054f87b8f8d3e45150f00b7b5675323aa36b36d324d087b9  /usr/local/bin/n" | sha256sum -c - \
    && chmod +x /usr/local/bin/n \
    && N_PREFIX=/usr/local n install "$NODE_VERSION" \
    && npm install -g pnpm@12.18.0 yarn@1.22.22 grunt-cli@1.5.0 \
    && npm cache clean --force \
    && rm -rf /usr/local/include/node /usr/local/share/doc/node /usr/local/share/man/man1/node.1 /usr/local/share/systemtap

# Late on purpose: the PHP install only symlinks to zz-lodev.ini, so editing it reruns just these layers.
COPY zz-lodev.ini /etc/lodev/zz-lodev.ini
# zz-lodev.ini prepends php.d/prepend.php: a placeholder so PHP runs in builds; the project mount hides it.
RUN mkdir -p /etc/lodev/php.d && touch /etc/lodev/php.d/prepend.php

COPY nginx-status.conf /etc/nginx/conf.d/lodev-status.conf
COPY apache-status.conf /etc/apache2/conf-enabled/lodev-status.conf
COPY healthcheck.sh /usr/local/bin/lodev-healthcheck
COPY xdebug.ini /etc/lodev/xdebug.ini
COPY supervisor/ /etc/lodev/supervisor/
COPY skel/ /etc/skel/
COPY entrypoint.sh /usr/local/bin/lodev-entrypoint
# The embedded files are 0644/0755: ssh refuses a config that is not private.
RUN chmod +x /usr/local/bin/lodev-entrypoint /usr/local/bin/lodev-healthcheck \
    && chmod 700 /etc/skel/.ssh && chmod 600 /etc/skel/.ssh/config

WORKDIR /app
ENTRYPOINT ["/usr/local/bin/lodev-entrypoint"]
