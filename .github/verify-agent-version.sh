#!/usr/bin/env bash

set -euxo pipefail

model_type="$1"
target_version="$2"
attempt=0
while true; do
  if [[ "$model_type" == "iaas" ]]; then
    UPDATED=$( (juju ssh -mcontroller 0 sudo cat /var/lib/juju/agents/machine-0/agent.conf || echo "") | yq -r '.upgradedToVersion' )
  elif [[ "$model_type" == "caas" ]]; then
    UPDATED=$( (juju ssh -mcontroller --container api-server 0 cat /var/lib/juju/agents/controller-0/agent.conf || echo "") | yq -r '.upgradedToVersion' )
  fi

  if [[ $UPDATED == $target_version* ]]; then
      break
  fi
  sleep 10
  attempt=$((attempt+1))
  if [ "$attempt" -eq 48 ]; then
      echo "Upgrade version check timed out"
      exit 1
  fi
done
