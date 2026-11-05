#!/bin/bash
# Installs and configures the given PHP versions from deb.sury.org.
# Used by the base image and by legacy images (Dockerfile.legacy).
set -euo pipefail

if [ $# -eq 0 ]; then
  echo "usage: lodev-install-php <version>..." >&2
  exit 1
fi

pkgs=()
for v in "$@"; do
  for ext in fpm cli bcmath curl gd intl mbstring mysql redis soap xml zip imagick xdebug; do
    pkgs+=("php$v-$ext")
  done
  # OPcache is built into PHP from 8.5; sury ships no php8.5-opcache.
  if dpkg --compare-versions "$v" lt 8.5; then
    pkgs+=("php$v-opcache")
  fi
done

# --error-on=any: fail here, not later with "Unable to locate package", when sury is unreachable.
apt-get update --error-on=any
apt-get install -y --no-install-recommends "${pkgs[@]}"

for v in "$@"; do
  phpdismod -v "$v" -s ALL xdebug
  sed -i 's|^listen = .*|listen = 127.0.0.1:9000|' "/etc/php/$v/fpm/pool.d/www.conf"
  printf 'clear_env = no\ncatch_workers_output = yes\n' >> "/etc/php/$v/fpm/pool.d/www.conf"
  sed -i 's|^error_log = .*|error_log = /proc/self/fd/2|' "/etc/php/$v/fpm/php-fpm.conf"
  for sapi in cli fpm; do
    ln -sf /etc/lodev/zz-lodev.ini "/etc/php/$v/$sapi/conf.d/99-lodev.ini"
  done
done

apt-get clean
rm -rf /var/lib/apt/lists/* /var/cache/apt/* /tmp/* /var/log/apt/* /var/log/dpkg.log
