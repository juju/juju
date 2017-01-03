#!/bin/bash
# Reset maas fabrics and vlans to 0, the defaults.
set -eux


ENV=$1

# Learn all the system_id and hostnames to lookup later.
ALL_SYSTEM_HOSTS=$(
    maas $ENV machines read |
    grep -P 'system_id|hostname' |
    sed -r ' N;s/\n/ /; s/( +"system_id": .[^,]+,)(.*)/\2 \1/; s, +"(system_id|hostname)": ,,g; s/"//g; s/,/@/; s/,//')
# Find the unwanted fabric, which is most likely the one with a number
# greater than 9.
FABRIC=$(
    maas $ENV fabrics read |
    sed -r '/\/fabrics\/[0-9][0-9]+/!d; s,.*/fabrics/([0-9][0-9]+)/.*,\1,')
# Learn the misconfigured interfaces by attempting to delete
# the unwanted fabric.
INTERFACES=$(
    maas $ENV fabric delete $FABRIC 2>&1 |
    sed -r 's,(^.*:|\(.*\)) ,,g; s, on ,@,; s/,//' ||
    true)
# Reset the fabric and vlan of each machine on the unwanted fabric.
for iface_machine in $INTERFACES; do
    iface=$(echo $iface_machine | cut -d @ -f1)
    machine=$(echo $iface_machine | cut -d @ -f2)
    system_id=$(
        echo "$ALL_SYSTEM_HOSTS" |
        grep $machine@ | cut -d @ -f2)
    maas $ENV interface update $system_id $iface fabric=0 vlan=0
done
# Delete the unwanted fabric.
maas $ENV fabric delete $FABRIC
