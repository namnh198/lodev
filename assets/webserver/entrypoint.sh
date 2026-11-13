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
done
mkdir -p /run/php

# Trust lodev's local CA so https://*.test works from inside the container.
if [ -f /etc/lodev/rootCA.pem ]; then
  cp /etc/lodev/rootCA.pem /usr/local/share/ca-certificates/lodev.crt
  update-ca-certificates >/dev/null
fi

# Run the services as the host user (created here so no image bakes in a UID).
if [ -n "${LODEV_USER:-}" ]; then
  group=$(getent group "$LODEV_GID" | cut -d: -f1)
  if [ -z "$group" ]; then
    groupadd -g "$LODEV_GID" "$LODEV_USER"
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
    useradd -m -d "/home/$LODEV_USER" -s /bin/bash -u "$LODEV_UID" -g "$group" "$LODEV_USER"
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

exec supervisord -n -c /etc/supervisor/supervisord.conf
