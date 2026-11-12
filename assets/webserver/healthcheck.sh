#!/bin/sh
# lodev web healthcheck (compose healthcheck): the project mount, the webserver and PHP behind it.
# Prints one line, e.g. "/var/www/html:OK nginx:OK php-fpm:OK phpstatus:OK"; exits 1 on any FAILED.
rc=0
check() { # name, command...
  name=$1; shift
  if "$@" >/dev/null 2>&1; then printf '%s:OK ' "$name"; else printf '%s:FAILED ' "$name"; rc=1; fi
}
# -c: run from /var/www/html, supervisorctl would read a project's own supervisord.conf first.
running() { supervisorctl -c /etc/supervisor/supervisord.conf status "$1" | grep -q RUNNING; }

# The project folder itself, not just any mount: its .lodev/config.yaml must be readable.
check /var/www/html test -r /var/www/html/.lodev/config.yaml
case "${LODEV_WEBSERVER:-nginx}" in
  none) check php php -r 'exit(0);' ;;
  *)
    check "$LODEV_WEBSERVER" running "$LODEV_WEBSERVER"
    check php-fpm running php-fpm
    # Through the webserver to php-fpm: lodev-status.conf serves it on 127.0.0.1 for any project config.
    check phpstatus curl -fsS --max-time 5 -o /dev/null http://127.0.0.1/phpstatus
    ;;
esac
echo
exit $rc
