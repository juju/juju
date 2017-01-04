#!/bin/bash -x

ENV=$1
BASE_NODE_NUM=$2
ADD_COUNT=$3
base_name='maas-node'
power_address='qemu+ssh://jenkins@10.0.200.1/system'
pool_path='/images/maaspool1'

if [[ ! -d $HOME/juju-ci-tools/maas ]]; then
    echo "\$HOME/juju-ci-tools/maas is required."
    exit 1;
fi

next_num=${BASE_NODE_NUM}
last_num=$((next_num + ADD_COUNT))
while (($next_num <= $last_num)); do
    mac_to_add=$(
        virsh dumpxml ${base_name}-${next_num} |
        grep "mac address" |
        head -2 |
        tail -1 |
        awk -F\' '{print $2}')
    maas ${ENV} machines create \
        architecture="amd64/generic" mac_addresses="${mac_to_add}" \
        hostname="${base_name}-${next_num}" power_type="virsh" \
        power_parameters_power_address="${power_address}" \
        power_parameters_power_id="${base_name}-${next_num}" \
        autodetect_nodegroup=1
    next_num=$((next_num+1))
done
