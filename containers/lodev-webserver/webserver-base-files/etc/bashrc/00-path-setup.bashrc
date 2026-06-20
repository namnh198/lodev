path_prepend() {
  case ":$PATH:" in
    *":$1:"*) ;;
    *) PATH="$1:$PATH" ;;
  esac
}
path_append() {
  case ":$PATH:" in
    *":$1:"*) ;;
    *) PATH="$PATH:$1" ;;
  esac
}

path_prepend "$HOME/n/bin"
path_prepend "$HOME/bin"
path_prepend "${LODEV_COMPOSER_ROOT:-/var/www/html}/vendor/bin"

path_append "${LODEV_COMPOSER_ROOT:-/var/www/html}/bin"
path_append "/mnt/lodev_default/commands/web"

unset -f path_prepend path_append

# And don't forget to export the new $PATH.
export PATH
