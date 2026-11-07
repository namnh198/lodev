FROM debian:trixie-slim

ARG PHP_VERSIONS
ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-eu", "-o", "pipefail", "-c"]

# Base packages and the deb.sury.org PHP repository; PHP itself is installed below.
RUN test -n "$PHP_VERSIONS" \
    && apt-get update --error-on=any \
    && apt-get install -y --no-install-recommends \
       ca-certificates curl nginx apache2 supervisor default-mysql-client git unzip sudo \
       nano vim-tiny sqlite3 \
    && curl -fsSL -o /tmp/sury-keyring.deb https://packages.sury.org/debsuryorg-archive-keyring.deb \
    && dpkg -i /tmp/sury-keyring.deb \
    && . /etc/os-release \
    && printf 'Types: deb\nURIs: https://packages.sury.org/php/\nSuites: %s\nComponents: main\nSigned-By: /usr/share/keyrings/deb.sury.org-php.gpg\n' \
       "$VERSION_CODENAME" > /etc/apt/sources.list.d/php.sources \
    && rm -f /etc/nginx/sites-enabled/default \
    && a2dismod -f mpm_prefork \
    && a2enmod mpm_event proxy_fcgi rewrite headers setenvif \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/* /tmp/* /var/log/apt/* /var/log/dpkg.log

COPY install-php.sh /usr/local/bin/lodev-install-php
RUN lodev-install-php $PHP_VERSIONS
# After the install: the install only symlinks to this file, so editing it skips the PHP layer.
COPY zz-lodev.ini /etc/lodev/zz-lodev.ini
# zz-lodev.ini prepends php.d/prepend.php: a placeholder so PHP runs in builds; the project mount hides it.
RUN mkdir -p /etc/lodev/php.d && touch /etc/lodev/php.d/prepend.php

COPY --from=composer:2 /usr/bin/composer /usr/local/bin/composer
# Mailpit's binary doubles as a sendmail replacement for PHP mail().
COPY --from=axllent/mailpit /mailpit /usr/local/bin/mailpit
RUN curl -fsSL -o /usr/local/bin/wp https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar \
    && curl -fsSL -o /usr/local/bin/n98-magerun2 https://files.magerun.net/n98-magerun2.phar \
    && chmod +x /usr/local/bin/wp /usr/local/bin/n98-magerun2

# Node.js via n, into /usr/local like a project's nodejs: (which reinstalls over it).
ARG NODE_VERSION=24
RUN curl -fsSL -o /usr/local/bin/n https://raw.githubusercontent.com/tj/n/v10.1.0/bin/n \
    && chmod +x /usr/local/bin/n \
    && N_PREFIX=/usr/local n install --cleanup "$NODE_VERSION"

COPY xdebug.ini /etc/lodev/xdebug.ini
COPY supervisor/ /etc/lodev/supervisor/
COPY entrypoint.sh /usr/local/bin/lodev-entrypoint
RUN chmod +x /usr/local/bin/lodev-entrypoint

WORKDIR /var/www/html
ENTRYPOINT ["/usr/local/bin/lodev-entrypoint"]
