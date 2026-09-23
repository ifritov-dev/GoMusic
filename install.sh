#!/bin/sh
# Installer for GoMusic (macOS, Linux, Windows via Git Bash).
# Builds the app and registers the `gomusic` command using the app's
# own first-run setup -- nothing OS-specific lives here on purpose.
set -eu
cd "$(dirname "$0")"

command -v go >/dev/null 2>&1 || {
	echo "Go not found -- install it first: https://go.dev/dl"
	exit 1
}

BIN=gomusic
case "$(uname -s 2>/dev/null)" in
	*MINGW* | *MSYS* | *CYGWIN* | *indows*) BIN=gomusic.exe ;;
esac

go build -o "$BIN" .

# First launch performs the self-install (symlink on macOS/Linux,
# user PATH on Windows). stdin is closed so the TUI quits at once
# instead of taking over the terminal.
"./$BIN" </dev/null >/dev/null 2>&1 || true

if command -v gomusic >/dev/null 2>&1; then
	echo "Done -- open a new terminal and just type: gomusic"
else
	echo "Built ./$BIN, but 'gomusic' is not on PATH in this shell."
	echo "Reopen the terminal and run ./$BIN once, or add ~/.local/bin to PATH:"
	echo '  export PATH="$HOME/.local/bin:$PATH"'
fi
