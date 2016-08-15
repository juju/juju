#!/bin/bash -x

add_count=8
base_name='maas-node'
base_node_num=1
maas_login_key='maas19'
power_address='qemu+ssh://ubuntu@10.0.30.1/system'
pool_path='/images/maaspool1'
base='/images'

[[ -d $base/maas ]] || { echo "\$base/maas is required."; exit 1; }
# Add the first node
sudo qemu-img create -f qcow2 ${pool_path}/${base_name}-1.1.qcow2 40G
sudo qemu-img create -f qcow2 ${pool_path}/${base_name}-1.2.qcow2 40G
virsh define $base/maas/maas-node-1.xml

## Clone additional nodes
next_num=${base_node_num}
last_num=$((next_num + add_count))
next_num=$((base_node_num + 1))
while (($next_num <= $last_num)); do
    virt-clone -o ${base_name}-${base_node_num} -n ${base_name}-${next_num} -f ${pool_path}/${base_name}-${next_num}.1.qcow2 -f ${pool_path}/${base_name}-${next_num}.2.qcow2
    mac_to_add=$(virsh dumpxml ${base_name}-${next_num} |grep "mac address" | head -2 | tail -1 | awk -F\' '{print $2}')
    maas ${maas_login_key} nodes new architecture="ppc64el/generic" mac_addresses="${mac_to_add}" hostname="${base_name}-${next_num}" power_type="virsh" power_parameters_power_address="${power_address}" power_parameters_power_id="${base_name}-${next_num}" autodetect_nodegroup=1
    next_num=$((next_num+1))

done

# Add the original node, which could not be running while cloning.
mac_to_add=$(virsh dumpxml ${base_name}-${base_node_num} |grep "mac address" | head -2 | tail -1 | awk -F\' '{print $2}')
maas ${maas_login_key} nodes new architecture="ppc64el/generic" mac_addresses="${mac_to_add}" hostname="${base_name}-${base_node_num}" power_type="virsh" power_parameters_power_address="${power_address}" power_parameters_power_id="${base_name}-${base_node_num}" autodetect_nodegroup=1
