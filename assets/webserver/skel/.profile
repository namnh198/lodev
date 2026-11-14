# ~/.profile: executed by the command interpreter for login shells.

# if running bash
if [ -n "$BASH_VERSION" ]; then
    # include .bashrc if it exists
    if [ -f "$HOME/.bashrc" ]; then
        . "$HOME/.bashrc"
    fi
fi

# set PATH so it includes user's private bin directories if they exist
for d in "$HOME/bin" "$HOME/.local/bin"; do
    if [ -d "$d" ]; then
        case ":$PATH:" in
            *":$d:"*) ;;
            *) PATH="$d:$PATH" ;;
        esac
    fi
done
unset d
