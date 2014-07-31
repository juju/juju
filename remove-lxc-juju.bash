#!/bin/bash
set -eux

EXIT_CODE=0
ERROR_WHEN_DIRTY='false'
while [[ "${1-}" != "" && $1 =~ ^-.*  ]]; do
    case $1 in
        --error-when-dirty)
            ERROR_WHEN_DIRTY="true"
            ;;
    esac
    shift
done


RUNNING_SERVICES=$(
    sudo lxc-ls --fancy |
    grep $USER-juju |
    grep RUNNING |
    cut -d ' ' -f 1)
[[ -n "$RUNNING_SERVICES" && $ERROR_WHEN_DIRTY == 'true' ]] && EXIT_CODE=1
for service in $RUNNING_SERVICES; do
    sudo lxc-stop -n $service
done


STOPPED_SERVICES=$(
    sudo lxc-ls --fancy |
    grep $USER-juju |
    grep STOPPED |
    cut -d ' ' -f 1)
[[ -n "$STOPPED_SERVICES" && $ERROR_WHEN_DIRTY == 'true' ]] && EXIT_CODE=1
for service in $RUNNING_SERVICES; do
    sudo lxc-destroy -n $service
    if [[ -e /etc/lxc/auto/$service ]]; then
        sudo rm -r /etc/lxc/auto/$service
    fi
done


RUNNING_TEMPLATES=$(
    sudo lxc-ls --fancy |
    grep juju-.*-template |
    grep RUNNING |
    cut -d ' ' -f 1)
[[ -n "$RUNNING_TEMPLATES" && $ERROR_WHEN_DIRTY == 'true' ]] && EXIT_CODE=1
for template in $RUNNING_TEMPLATES; do
    sudo lxc-stop -n $template
done


set +e
STALE_LOCKS=$(find /var/lib/juju/locks -maxdepth 1 -type d -name 'juju-*')
set -e
[[ -n "$STALE_LOCKS" && $ERROR_WHEN_DIRTY == 'true' ]] && EXIT_CODE=1
for lock in $STALE_LOCKS; do
    sudo rm -r $lock
done
echo "cleaned"
exit $EXIT_CODE
