#!/bin/sh
# Fixed-port integration tests must never share the owner's loopback namespace.
set -eu
if [ "$#" -eq 0 ]; then
    echo 'Usage: with-isolated-network.sh command [args...]' >&2
    exit 2
fi
companion_host_network=$(readlink /proc/self/ns/net)
export STEAM_SAGE_TEST_HOST_NETWORK="$companion_host_network"
exec unshare --user --map-current-user --keep-caps --net sh -eu -c '
    ip link set lo up
    export STEAM_SAGE_ISOLATED_NETWORK=1
    exec "$@"
' sh "$@"
