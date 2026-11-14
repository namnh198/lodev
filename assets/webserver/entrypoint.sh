#!/bin/sh
set -e

# userns-remap: begin
# dockerd's userns-remap stores image files under the remapped root's UID; with userns_mode: host
# that UID shows here, and sudo refuses to run unless its files are root's. A no-op otherwise.
if [ "$(stat -c %u /usr/bin/sudo)" != 0 ]; then
  chown -R 0:0 /usr/bin/sudo /etc/sudo.conf /etc/sudoers /etc/sudoers.d /usr/libexec/sudo
  chmod 4755 /usr/bin/sudo
fi
# userns-remap: end

if [ ! -x "/usr/sbin/php-fpm${LODEV_PHP}" ]; then
  echo "unsupported LODEV_PHP: ${LODEV_PHP}" >&2
  exit 1
fi
update-alternatives --quiet --set php "/usr/bin/php${LODEV_PHP}"
ln -sf "/usr/sbin/php-fpm${LODEV_PHP}" /usr/local/sbin/php-fpm
for sapi in cli fpm; do
  d="/etc/php/${LODEV_PHP}/${sapi}/conf.d"
  rm -f "$d"/99-zz-*
  for f in /etc/lodev/php.d/*.ini; do
    [ -e "$f" ] && ln -sf "$f" "$d/99-zz-$(basename "$f")"
  done
  if [ "${LODEV_OPCACHE_VALIDATE:-}" = 0 ]; then
    echo "opcache.validate_timestamps=0" > /etc/lodev/opcache-off.ini
    ln -sf /etc/lodev/opcache-off.ini "$d/99-zz-lodev-opcache-off.ini"
  fi
done
mkdir -p /run/php

# Trust lodev's local CA so https://*.test works from inside the container.
if [ -f /etc/lodev/rootCA.pem ]; then
  cp /etc/lodev/rootCA.pem /usr/local/share/ca-certificates/lodev.crt
  # OrbStack injects bundle files into /etc/ssl/certs that rehash skips with a harmless warning on
  # every start. Capture the output (sh -e has no pipefail): a failure shows all of it, a success
  # drops stdout and the rehash skip warnings and shows any other stderr line.
  ca_out=$(mktemp)
  ca_err=$(mktemp)
  if ! update-ca-certificates >"$ca_out" 2>"$ca_err"; then
    cat "$ca_out" "$ca_err" >&2
    exit 1
  fi
  grep -v '^rehash: warning: skipping' "$ca_err" >&2 || true
  rm -f "$ca_out" "$ca_err"
fi

# Run the services as the host user (created here so no image bakes in a UID).
if [ -n "${LODEV_USER:-}" ]; then
  group=$(getent group "$LODEV_GID" | cut -d: -f1)
  if [ -z "$group" ]; then
    groupadd -K GID_MIN=1 -K GID_MAX=4294967294 -g "$LODEV_GID" "$LODEV_USER"
    group=$LODEV_USER
  elif [ "$group" != "$LODEV_USER" ] && [ "$group" != root ] && [ "$group" != www-data ] &&
    ! getent group "$LODEV_USER" >/dev/null; then
    # The host GID names another group here (macOS staff is 20, Debian's dialout): show the user's
    # name instead. Only the name changes; files and permissions go by the number.
    groupmod -n "$LODEV_USER" "$group"
    group=$LODEV_USER
  fi
  by_name=$(getent passwd "$LODEV_USER" | cut -d: -f3)
  by_uid=$(getent passwd "$LODEV_UID" | cut -d: -f1)
  if [ -n "$by_name" ] && [ "$by_name" != "$LODEV_UID" ]; then
    echo "user $LODEV_USER exists with UID $by_name, not $LODEV_UID" >&2
    exit 1
  fi
  if [ -n "$by_uid" ] && [ "$by_uid" != "$LODEV_USER" ]; then
    echo "UID $LODEV_UID belongs to $by_uid, not $LODEV_USER" >&2
    exit 1
  fi
  if [ -z "$by_name" ]; then
    # -K widens the system's 1000-60000 range so macOS (501) and high LDAP UIDs raise no warning.
    useradd -K UID_MIN=1 -K UID_MAX=4294967294 -m -d "/home/$LODEV_USER" -s /bin/bash -u "$LODEV_UID" -g "$group" "$LODEV_USER"
  fi
  # The home may be a mounted, persistent folder: useradd skips /etc/skel then. Copy what is
  # missing as the user (no chown -R over caches), never overwriting the user's files.
  runuser -u "$LODEV_USER" -- cp -r --update=none /etc/skel/. "/home/$LODEV_USER/"
  printf '%s ALL=(ALL) NOPASSWD:ALL\n' "$LODEV_USER" > /etc/sudoers.d/lodev
  chmod 0440 /etc/sudoers.d/lodev
  sed -i "s/^user .*;/user $LODEV_USER $group;/" /etc/nginx/nginx.conf
  sed -i -e "s/^user = .*/user = $LODEV_USER/" -e "s/^group = .*/group = $group/" "/etc/php/$LODEV_PHP/fpm/pool.d/www.conf"
  sed -i -e "s/^export APACHE_RUN_USER=.*/export APACHE_RUN_USER=$LODEV_USER/" -e "s/^export APACHE_RUN_GROUP=.*/export APACHE_RUN_GROUP=$group/" /etc/apache2/envvars
fi

# A new volume is root-owned and PHP runs as the host user. Top level only: a seeded volume keeps
# the host's ownership, and a recursive chown of a large tree on every start would be slow.
for d in ${LODEV_VOLUME_DIRS:-}; do
  t="$LODEV_WORKDIR/$d"
  if [ "$(stat -c %u "$t")" = 0 ]; then
    chown "$LODEV_UID:$LODEV_GID" "$t"
  fi
done

rm -f /etc/supervisor/conf.d/*.conf
case "${LODEV_WEBSERVER:-nginx}" in
  nginx)
    cp /etc/lodev/supervisor/php-fpm.conf /etc/lodev/supervisor/nginx.conf /etc/supervisor/conf.d/
    cp /etc/lodev/webserver.conf /etc/nginx/conf.d/default.conf
    ;;
  apache)
    cp /etc/lodev/supervisor/php-fpm.conf /etc/lodev/supervisor/apache.conf /etc/supervisor/conf.d/
    cp /etc/lodev/webserver.conf /etc/apache2/sites-enabled/000-default.conf
    ;;
  none) ;;
  *)
    echo "unknown LODEV_WEBSERVER: $LODEV_WEBSERVER" >&2
    exit 1
    ;;
esac
if [ "${LODEV_CRON:-}" = 1 ]; then
  cp /etc/lodev/supervisor/cron.conf /etc/supervisor/conf.d/
fi
if [ "${LODEV_DAEMONS:-}" = 1 ]; then
  cp /etc/lodev/daemons/daemons.conf /etc/supervisor/conf.d/
fi

exec supervisord -n -c /etc/supervisor/supervisord.conf
