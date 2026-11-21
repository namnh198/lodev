#!/bin/sh
# lodev web healthcheck (compose healthcheck): the project mount, the webserver and PHP (or Next) behind it.
# Prints one line, e.g. "/app:OK nginx:OK php-fpm:OK phpstatus:OK"; exits 1 on any FAILED.
rc=0
check() { # name, command...
  name=$1; shift
  if "$@" >/dev/null 2>&1; then printf '%s:OK ' "$name"; else printf '%s:FAILED ' "$name"; rc=1; fi
}
# -c: run from the project folder, supervisorctl would read a project's own supervisord.conf first.
running() { supervisorctl -c /etc/supervisor/supervisord.conf status "$1" | grep -q RUNNING; }

# The project folder itself, not just any mount: its .lodev/config.yaml must be readable.
dir=${LODEV_WORKDIR:-/app}
check "$dir" test -r "$dir/.lodev/config.yaml"
case "${LODEV_WEBSERVER:-nginx}" in
  none) check php php -r 'exit(0);' ;;
  *)
    check "$LODEV_WEBSERVER" running "$LODEV_WEBSERVER"
    check php-fpm running php-fpm
    if [ -n "${LODEV_NEXTJS_PORT:-}" ]; then
      # nginx proxies to Next: until it listens, the site is a 502. A bare connect, no request,
      # so next dev compiles and logs nothing.
      check nextjs bash -c ': </dev/tcp/127.0.0.1/$1' _ "$LODEV_NEXTJS_PORT"
    else
      # Through the webserver to php-fpm: lodev-status.conf serves it on 127.0.0.1 for any project config.
      check phpstatus curl -fsS --max-time 5 -o /dev/null http://127.0.0.1/phpstatus
    fi
    ;;
esac
echo
exit $rc
