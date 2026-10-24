#!/bin/sh
set -eu
companion_package_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
"$companion_package_dir/steam-sage-companion" install "$@"
exec "$HOME/.local/bin/steam-sage-companion" launch "$@"
