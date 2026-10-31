FROM node:24-slim AS node
FROM debian:trixie-slim

ARG PHP_VERSIONS
ARG UID=1000
ARG GID=1000
ENV DEBIAN_FRONTEND=noninteractive
SHELL ["/bin/bash", "-eu", "-o", "pipefail", "-c"]

# Base packages and the deb.sury.org PHP repository; PHP itself is installed below.
RUN test -n "$PHP_VERSIONS" \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates curl nginx apache2 supervisor default-mysql-client git unzip \
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

COPY zz-lodev.ini /etc/lodev/zz-lodev.ini
COPY install-php.sh /usr/local/bin/lodev-install-php
RUN lodev-install-php $PHP_VERSIONS

COPY --from=composer:2 /usr/bin/composer /usr/local/bin/composer
# Mailpit's binary doubles as a sendmail replacement for PHP mail().
COPY --from=axllent/mailpit /mailpit /usr/local/bin/mailpit
RUN curl -fsSL -o /usr/local/bin/wp https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar \
    && chmod +x /usr/local/bin/wp

COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

# -o: macOS GID 20 (staff) already exists in Debian as dialout.
RUN usermod -o -u ${UID} www-data \
    && groupmod -o -g ${GID} www-data \
    && chown -R www-data:www-data /var/www

COPY xdebug.ini /etc/lodev/xdebug.ini
COPY supervisor/ /etc/lodev/supervisor/
COPY entrypoint.sh /usr/local/bin/lodev-entrypoint
RUN chmod +x /usr/local/bin/lodev-entrypoint

WORKDIR /var/www/html
ENTRYPOINT ["/usr/local/bin/lodev-entrypoint"]
