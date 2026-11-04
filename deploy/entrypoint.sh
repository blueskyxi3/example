#!/bin/sh
set -eu

if [ -n "${AIHUB_PLUGIN_CONFIG:-}" ]; then
  # Kubernetes mounts the supplied config read-only; runtime callbacks need a writable copy.
  if [ "$AIHUB_PLUGIN_CONFIG" != /data/plugin/config.yaml ]; then
    cp "$AIHUB_PLUGIN_CONFIG" /data/plugin/config.yaml
  fi
  chmod 600 /data/plugin/config.yaml
  # Use the listen address/port supplied by the deployer instead of the image default.
  unset EXAMPLE_SERVER_ADDRESS
fi

exec /usr/local/bin/example "$@"
