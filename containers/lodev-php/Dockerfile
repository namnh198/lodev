### --------------- END LODEV-PHP-DEV IMAGE ------------------ ###
FROM ubuntu:24.04 AS lodev-php-base

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG PHP_VERSIONS="php5.6 php7.0 php7.1 php7.2 php7.3 php7.4 php8.0 php8.1 php8.2 php8.3 php8.4 php8.5"
ARG PHP_DEFAULT_VERSION="8.4"
ARG N_PREFIX=/usr/local
ARG XDEBUG_MODE="off"

ENV DEBIAN_FRONTEND=noninteractive
ENV LODEV_PHP_VERSION=${PHP_DEFAULT_VERSION}
ENV NODE_VERSION=lts
ENV PHP_INI="/etc/php/${PHP_DEFAULT_VERSION}/fpm/php.ini"
ENV COMPOSE_ALLOW_SUPERUSER=1
ENV COMPOSER_PROCESS_TIMEOUT=2000

SHELL [ "/bin/bash", "-eu", "-o", "pipefail", "-c" ]

RUN apt-get -qq update && apt-get -qq install --no-install-recommends --no-install-suggests -y \
  software-properties-common \
  ca-certificates \
  lsb-release \
  apt-transport-https \
  gnupg \
  curl \
  zip \
  unzip \
  git \
  procps \
  tzdata-legacy \
  wget \
  file \
  ghostscript \
  imagemagick \
  graphicsmagick \
  jq \
  msmtp \
  sqlite3 \
  lsof

RUN add-apt-repository ppa:ondrej/php -y && apt-get -qq update

# install yq to parse yaml files in bash scripts
RUN url="https://github.com/mikefarah/yq/releases/latest/download/yq_linux_${TARGETPLATFORM#linux/}"; wget ${url} -q -O /usr/bin/yq && chmod +x /usr/bin/yq

# install xdebugctl to easily enable/disable xdebug
RUN ARCH="${TARGETPLATFORM#linux/}"; if [ "$ARCH" = "amd64" ]; then ARCH=""; else ARCH="-${ARCH}"; fi; wget -q https://xdebug.org/files/binaries/xdebugctl${ARCH} -O /usr/local/bin/xdebugctl && chmod +x /usr/local/bin/xdebugctl

COPY generic-files/install_php_extensions.sh /install_php_extensions
COPY generic-files/etc/php-packages.yml /etc/php-packages.yml

RUN chmod ugo+rx /install_php_extensions

# Install PHP & Common PHP extensions
RUN for v in ${PHP_VERSIONS}; do \
  /install_php_extensions ${v} "${TARGETPLATFORM#linux/}"; \
  done

# Set the default PHP version
RUN update-alternatives --set php /usr/bin/php${PHP_DEFAULT_VERSION}

ADD generic-files /

RUN chmod ugo+rwx /usr/local/bin/*

RUN touch /var/log/php-fpm.log && \
  chmod ugo+rw /var/log/php-fpm.log && \
  chmod ugo+rwx /var/run && \
  chmod ugo+rx /usr/local/bin/* && \
  ln -sf /usr/sbin/php-fpm${PHP_DEFAULT_VERSION} /usr/sbin/php-fpm

RUN phpdismod xdebug

# install composer
RUN curl -L --fail -o /usr/local/bin/composer -sSL https://getcomposer.org/composer-stable.phar && chmod ugo+wx /usr/local/bin/composer

# install symfony cli
RUN curl -sS https://get.symfony.com/cli/installer | bash && mv /root/.symfony5/bin/symfony /usr/local/bin/symfony

# wp-cli for wordpress
RUN curl --fail -sSL -o /usr/local/bin/wp-cli -O https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar && chmod +x /usr/local/bin/wp-cli && ln -sf /usr/local/bin/wp-cli /usr/local/bin/wp

# magerun for openmage
RUN curl --fail -sSL https://files.magerun.net/n98-magerun-latest.phar -o /usr/local/bin/magerun && chmod 755 /usr/local/bin/magerun
RUN curl --fail -sSL https://raw.githubusercontent.com/netz98/n98-magerun/master/res/autocompletion/bash/n98-magerun.phar.bash -o /etc/bash_completion.d/n98-magerun.phar

# magerun2 for magento
RUN curl --fail -sSL https://files.magerun.net/n98-magerun2-latest.phar -o /usr/local/bin/magerun2 && chmod 755 /usr/local/bin/magerun2
RUN curl --fail -sSL https://raw.githubusercontent.com/netz98/n98-magerun2/master/res/autocompletion/bash/n98-magerun2.phar.bash -o /etc/bash_completion.d/n98-magerun2.phar

# Install n package to manage node versions and install the latest stable node version (currently 24)
RUN curl -L --fail -o /usr/local/bin/n -sSL https://raw.githubusercontent.com/tj/n/master/bin/n && chmod +x /usr/local/bin/n

RUN n install --cleanup "${NODE_VERSION}"
RUN npm install --global pnpm

RUN ln -sf /usr/sbin/php-fpm${LODEV_PHP_VERSION} /usr/sbin/php-fpm
RUN mkdir -p /run/php && chown -R www-data:www-data /run
RUN chmod 777 /var/lib/php/sessions

RUN apt-get -qq autoremove -y && apt-get -qq clean -y && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/share/doc/* /usr/share/man/*
### --------------- END LODEV-PHP-DEV IMAGE ------------------ ###

### --------------- LODEV-PHP IMAGE ------------------ ###
FROM scratch AS lodev-php
ARG XDEBUG_MODE="off"

SHELL [ "/bin/bash", "-eu", "-o", "pipefail", "-c" ]

COPY --from=lodev-php-base / /

EXPOSE 8080 8585

CMD [ "/usr/sbin/php-fpm", "-F" ]
### --------------- END LODEV-PHP IMAGE ------------------ ###
