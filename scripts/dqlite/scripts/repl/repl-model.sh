#!/bin/bash

set -e

args=("$@")
DB=${args[0]:-0}

list_models() {
    source "$(dirname $0)/repl-list-models.sh"
}

if [ "$DB" != "0" ]; then
    DB_NAME=$(echo "$(list_models)" | head -n $(($DB+1)) | tail -n 1 | awk '{print $1}')
fi

source "$(dirname $0)/repl.sh"

