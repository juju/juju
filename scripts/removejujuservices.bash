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
done

echo "removing /var/lib/juju/db/*"
rm -rf /var/lib/juju/db/*

echo "removing /var/lib/juju/raft/*"
rm -rf /var/lib/juju/raft/*

echo "removing /var/run/juju/*"
rm -rf /var/run/juju/*
