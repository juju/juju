#!/bin/bash
# Prepares the state-server and remote machine,
# then adds the remote machine to the environment.
#
# add-remote-machine.bash env path/ssh-key user@remote_private_ip
#
# Your host must have access via ssh to the remote *private* ip.
# The state-server will only provision machines with private IPs;
# the agent will only talk to the state-server by private dns name.
# Your host will be used to start the provisioning. your hosts ssh
# rules must permit access to the remote host via its private ip.

set -eu

check_access() {
    name=$1
    url=$2
    echo "Checking $USER_AT_HOST can access $name at $url..."
    set +e
    result=$(ssh -i $SSH_KEY $USER_AT_HOST \
        curl --connect-timeout 5 --silent -k --head $url || echo "fail")
    set -e
    result=$(echo "$result" | tail -1)
    if [[ $result == "fail" ]]; then
        echo "...FAIL"
        NETWORK_ACCESS="false"
    else
        echo "...OK"
    fi
}


if [[ "$1" == "--dry-run" ]]; then
    DRY_RUN="true"
    shift
else
    DRY_RUN="false"
fi
MODEL=$1
SSH_KEY=$2
USER_AT_HOST=$3
NETWORK_ACCESS="true"

private_dns_name=$(
    juju ssh -m controller 0 'echo "$(hostname).$(dnsdomainname)"')
public_ip=$(
    juju status -m controller --format yaml 0 |
    sed -e '/dns-name/!d; s/.*: //')
api_port=$(juju controller-config api-port)
echo "Controller public address: $public_ip"
echo "Controller private address: $private_dns_name"
check_access "controller" https://$public_ip:$api_port

if [[ $DRY_RUN == "true" || $NETWORK_ACCESS == 'false' ]]; then
    exit
fi

echo "Adding controllers's private dns name to $USER_AT_HOST:/etc/hosts"
ssh -i $SSH_KEY $USER_AT_HOST \
    "echo '$public_ip $private_dns_name' | sudo tee -a /etc/hosts"

echo "Adding $USER_AT_HOST to $MODEL"
juju --show-log add-machine -m $MODEL ssh:$USER_AT_HOST
