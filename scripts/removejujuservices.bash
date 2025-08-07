#!/bin/bash

# WARNING
# This script will clean a host previously used to run a Juju controller/machine.
# Running this on a live installation will render Juju inoperable.

for path_to_unit in $(ls /etc/systemd/system/juju*); do
  echo "removing juju service: $path_to_unit"
  unit=$(basename "$path_to_unit")
  systemctl stop "$unit"
  systemctl disable "$unit"
  systemctl daemon-reload
  rm -f "$path_to_unit"
done

echo "removing /var/lib/juju/tools/*"
rm -rf /var/lib/juju/tools/*

echo "removing /var/lib/juju/dqlite/*"
rm -rf /var/lib/juju/dqlite/*

echo "removing /var/lib/juju/objecstore/*"
rm -rf /var/lib/juju/objecstore/*

echo "removing /var/run/juju/*"
rm -rf /var/run/juju/*
