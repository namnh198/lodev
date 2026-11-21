#!/bin/bash
# Installs and configures the given PHP versions from deb.sury.org.
# Used by the base image and by legacy images (Dockerfile.legacy).
set -euo pipefail

if [ $# -eq 0 ]; then
  echo "usage: lodev-install-php <version>..." >&2
  exit 1
fi

# php-pear (pecl) with the PHP packages, once the sury source is set up (apt picks the newest of Debian's and sury's).
pkgs=(php-pear)
for v in "$@"; do
  for ext in fpm cli bcmath curl gd intl mbstring mysql soap xml zip imagick dev; do
    pkgs+=("php$v-$ext")
  done
  # shortcut: sury has no php8.6-xdebug or php8.6-redis yet (Xdebug 3.5 stops at 8.5); drop the check once it does.
  if dpkg --compare-versions "$v" lt 8.6; then
    pkgs+=("php$v-redis" "php$v-xdebug")
  fi
  # OPcache is built into PHP from 8.5; sury ships no php8.5-opcache.
  if dpkg --compare-versions "$v" lt 8.5; then
    pkgs+=("php$v-opcache")
  fi
done

# --error-on=any: fail here, not later with "Unable to locate package", when sury is unreachable.
apt-get update --error-on=any
apt-get install -y --no-install-recommends "${pkgs[@]}"

for v in "$@"; do
  # The PHP2Shell fixes (CVE-2026-109618/109619): 8.2+ upstream, 7.4-8.1 as sury backports.
  case $v in
    7.4) min=1:7.4.33-31 ;;
    8.0) min=1:8.0.30-25 ;;
    8.1) min=8.1.34-9 ;;
    8.2) min=8.2.35 ;;
    8.3) min=8.3.36 ;;
    8.4) min=8.4.27 ;;
    8.5) min=8.5.12 ;;
    8.6) min=8.6.0 ;;
    *) echo "lodev-install-php: no patched minimum known for PHP $v" >&2; exit 1 ;;
  esac
  have=$(dpkg-query -W -f '${Version}' "php$v-cli")
  if ! dpkg --compare-versions "$have" ge "$min"; then
    echo "lodev-install-php: php$v-cli $have is older than the patched $min" >&2
    exit 1
  fi
  if [ -e "/etc/php/$v/mods-available/xdebug.ini" ]; then
    phpdismod -v "$v" -s ALL xdebug
  fi
  sed -i 's|^listen = .*|listen = 127.0.0.1:9000|' "/etc/php/$v/fpm/pool.d/www.conf"
  # ondemand: no idle workers; up to 20 for the parallel requests of an admin or a GraphQL page.
  sed -i -e 's|^pm = .*|pm = ondemand|' -e 's|^pm.max_children = .*|pm.max_children = 20|' \
    -e 's|^;*pm.process_idle_timeout = .*|pm.process_idle_timeout = 10s|' "/etc/php/$v/fpm/pool.d/www.conf"
  # pm.status_path: the healthcheck's /phpstatus (served on 127.0.0.1 only, see *-status.conf).
  printf 'clear_env = no\ncatch_workers_output = yes\npm.status_path = /phpstatus\n' >> "/etc/php/$v/fpm/pool.d/www.conf"
  sed -i 's|^error_log = .*|error_log = /proc/self/fd/2|' "/etc/php/$v/fpm/php-fpm.conf"
  for sapi in cli fpm; do
    ln -sf /etc/lodev/zz-lodev.ini "/etc/php/$v/$sapi/conf.d/99-lodev.ini"
  done
done

# apt's lists and .debs are BuildKit cache mounts (see the Dockerfiles): kept for the next build.
rm -rf /tmp/* /var/log/apt/* /var/log/dpkg.log
