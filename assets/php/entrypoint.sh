#!/bin/sh
set -e

# Trust lodev's local CA so https://*.test works from inside the container.
if [ -f /etc/lodev/rootCA.pem ]; then
  cp /etc/lodev/rootCA.pem /usr/local/share/ca-certificates/lodev.crt
  update-ca-certificates >/dev/null
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

exec supervisord -n -c /etc/supervisor/supervisord.conf
