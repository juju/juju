#!/bin/bash -x

ENV=$1
BASE_NODE_NUM=$2
ADD_COUNT=$3
BASE_NAME=${4:-maas-node}
POWER_ADDRESS=${5:-qemu+ssh://jenkins@10.0.200.1/system}

if [[ ! -d $HOME/juju-ci-tools/maas ]]; then
    echo "\$HOME/juju-ci-tools/maas is required."
    exit 1;
fi

next_num=${BASE_NODE_NUM}
last_num=$((next_num + ADD_COUNT))
while (($next_num <= $last_num)); do
    mac_to_add=$(
        virsh dumpxml ${BASE_NAME}-${next_num} |
        grep "mac address" |
        head -2 |
        tail -1 |
        awk -F\' '{print $2}')
    maas ${ENV} machines create \
        architecture="amd64/generic" mac_addresses="${mac_to_add}" \
        hostname="${BASE_NAME}-${next_num}" power_type="virsh" \
        power_parameters_power_address="${POWER_ADDRESS}" \
        power_parameters_power_id="${BASE_NAME}-${next_num}" \
        autodetect_nodegroup=1
    next_num=$((next_num+1))
done
