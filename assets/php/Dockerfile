ARG PHP_VERSION=8.3
FROM node:24-slim AS node
FROM php:${PHP_VERSION}-fpm

ARG UID=1000
ARG GID=1000
ARG DEBIAN_FRONTEND=noninteractive

# Images of EOL PHP versions (7.4/8.0) are frozen on archived Debian releases;
# deb.debian.org still answers for them but 404s on packages, so switch up front.
RUN . /etc/os-release \
    && if [ "$VERSION_CODENAME" = buster ] || [ "$VERSION_CODENAME" = bullseye ]; then \
         sed -i -e 's|deb.debian.org|archive.debian.org|g' -e '/-updates/d' /etc/apt/sources.list \
         && echo 'Acquire::Check-Valid-Until "false";' > /etc/apt/apt.conf.d/99archive; \
       fi \
    && apt-get update \
    && apt-get install -y --no-install-recommends \
       nginx apache2 supervisor default-mysql-client git unzip curl ca-certificates \
    && rm -rf /var/lib/apt/lists/* /etc/nginx/sites-enabled/default \
    && a2dismod -f mpm_prefork \
    && a2enmod mpm_event proxy_fcgi rewrite headers setenvif

COPY --from=mlocati/php-extension-installer /usr/bin/install-php-extensions /usr/local/bin/
RUN install-php-extensions bcmath gd intl pdo_mysql mysqli zip soap xsl sockets opcache exif redis xdebug \
    && mv /usr/local/etc/php/conf.d/docker-php-ext-xdebug.ini /usr/local/etc/php/xdebug.ini.disabled

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
    && chown -R www-data:www-data /var/www \
    && echo 'clear_env = no' >> /usr/local/etc/php-fpm.d/zz-docker.conf

COPY zz-lodev.ini /usr/local/etc/php/conf.d/zz-lodev.ini
COPY xdebug.ini /etc/lodev/xdebug.ini
COPY supervisor/ /etc/lodev/supervisor/
COPY entrypoint.sh /usr/local/bin/lodev-entrypoint
RUN chmod +x /usr/local/bin/lodev-entrypoint

WORKDIR /var/www/html
ENTRYPOINT ["/usr/local/bin/lodev-entrypoint"]
