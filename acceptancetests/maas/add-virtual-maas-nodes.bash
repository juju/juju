#!/bin/bash -x

add_count=24
base_name='maas-node'
base_node_num=1
maas_login_key='vmaas'
power_address='qemu+ssh://jenkins@10.0.200.1/system'
pool_path='/images/maaspool1'

[[ -d $HOME/juju-ci-tools/maas ]] || { echo "\$HOME/juju-ci-tools/maas is required."; exit 1; }
# Add the first node
sudo qemu-img create -f qcow2 ${pool_path}/${base_name}-1.1.qcow2 40G
sudo qemu-img create -f qcow2 ${pool_path}/${base_name}-1.2.qcow2 40G
virsh define $HOME/juju-ci-tools/maas/maas-node-1.xml

## Clone additional nodes
next_num=${base_node_num}
last_num=$((next_num + add_count))
next_num=$((base_node_num + 1))
while (($next_num <= $last_num)); do
    virt-clone -o ${base_name}-${base_node_num} -n ${base_name}-${next_num} -f ${pool_path}/${base_name}-${next_num}.1.qcow2 -f ${pool_path}/${base_name}-${next_num}.2.qcow2
    mac_to_add=$(virsh dumpxml ${base_name}-${next_num} |grep "mac address" | head -2 | tail -1 | awk -F\' '{print $2}')
    maas ${maas_login_key} nodes new architecture="amd64/generic" mac_addresses="${mac_to_add}" hostname="${base_name}-${next_num}" power_type="virsh" power_parameters_power_address="${power_address}" power_parameters_power_id="${base_name}-${next_num}" autodetect_nodegroup=1
    next_num=$((next_num+1))

done

# Add the original node, which could not be running while cloning.
mac_to_add=$(virsh dumpxml ${base_name}-${base_node_num} |grep "mac address" | head -2 | tail -1 | awk -F\' '{print $2}')
maas ${maas_login_key} nodes new architecture="amd64/generic" mac_addresses="${mac_to_add}" hostname="${base_name}-${base_node_num}" power_type="virsh" power_parameters_power_address="${power_address}" power_parameters_power_id="${base_name}-${base_node_num}" autodetect_nodegroup=1

# Add some nodes centos can use.
base_name='maas-node-first-NIC'
add_count=4
next_num=${base_node_num}
last_num=$((next_num + add_count))
next_num=$((base_node_num + 1))

# Add the first node
sudo qemu-img create -f qcow2 ${pool_path}/${base_name}-1.1.qcow2 40G
sudo qemu-img create -f qcow2 ${pool_path}/${base_name}-1.2.qcow2 40G
virsh define $HOME/juju-ci-tools/maas/maas-node-first-NIC-1.xml

maas ${maas_login_key} tags new name=MAAS_NIC_1 comment="MAAS managed through NIC 1."

while (($next_num <= $last_num)); do
    virt-clone -o ${base_name}-${base_node_num} -n ${base_name}-${next_num} -f ${pool_path}/${base_name}-${next_num}.1.qcow2 -f ${pool_path}/${base_name}-${next_num}.2.qcow2
    mac_to_add=$(virsh dumpxml ${base_name}-${next_num} |grep "mac address" | head -2 | tail -1 | awk -F\' '{print $2}')
    maas ${maas_login_key} nodes new architecture="amd64/generic" mac_addresses="${mac_to_add}" hostname="${base_name}-${next_num}" power_type="virsh" power_parameters_power_address="${power_address}" power_parameters_power_id="${base_name}-${next_num}" autodetect_nodegroup=1
    system_id=$(maas ${maas_login_key} nodes list mac_address="${mac_to_add}" | grep system_id | awk '{print $2}'| tr -d \")
    maas ${maas_login_key} tag update-nodes MAAS_NIC_1 add="$system_id"
    next_num=$((next_num+1))
done

# Add the original node, which could not be running while cloning.
mac_to_add=$(virsh dumpxml ${base_name}-${base_node_num} |grep "mac address" | head -2 | tail -1 | awk -F\' '{print $2}')
maas ${maas_login_key} nodes new architecture="amd64/generic" mac_addresses="${mac_to_add}" hostname="${base_name}-${base_node_num}" power_type="virsh" power_parameters_power_address="${power_address}" power_parameters_power_id="${base_name}-${base_node_num}" autodetect_nodegroup=1

