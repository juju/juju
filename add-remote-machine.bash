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
        curl --connect-timeout 5 --silent --head $url || echo "fail")
    set -e
    result=$(echo "$result" | tail -1)
    if [[ $result == "fail" ]]; then
        echo "...FAIL"
        NETWORK_ACCESS="false"
    else
        echo "...OK"
    fi
}


check_url_access() {
    option=$1
    option_url=$(juju get-env -e $ENV $option | sed -e 's,/tools$,,')
    if [[ -n "$option_url" ]]; then
        check_access $option $option_url
    else
        echo "! You must verify that $USER_AT_HOST can access $option"
    fi
}


if [[ "$1" == "--dry-run" ]]; then
    DRY_RUN="true"
    shift
else
    DRY_RUN="false"
fi
ENV=$1
SSH_KEY=$2
USER_AT_HOST=$3
NETWORK_ACCESS="true"

private_dns_name=$(juju ssh -e $ENV 0 'echo "$(hostname).$(dnsdomainname)"')
public_dns_name=$(juju status -e $ENV | sed -r '/dns-name/!d; 1,1!d; s/.*: (.*)/\1/')
public_ip=$(dig "$public_dns_name" | sed -r '/^;/d; /IN/!d; s/.*A (.*)/\1/;')
api_port=$(juju get-env -e $ENV api-port)
echo "State-server public address: $public_ip $public_dns_name"
echo "State-server private address: $private_dns_name"
check_access "state-server" http://$public_ip:$api_port
check_url_access image-metadata-url
check_url_access tools-metadata-url

echo "Checking $USER_AT_HOST can access the cloud provider's storage"
provider=$(juju get-env -e $ENV "type")
if [[ $provider == "ec2" ]]; then
    control_bucket=$(juju get-env -e $ENV control-bucket)
    check_access "s3" http://s3.amazon.com/$control_bucket
else
    echo "! You must verify that $USER_AT_HOST can access the cloud storage."
fi


if [[ $DRY_RUN == "true" || $NETWORK_ACCESS == 'false' ]]; then
    exit
fi

echo "Adding state-server's private dns name to $USER_AT_HOST:/etc/hosts"
ssh -i $SSH_KEY $USER_AT_HOST \
    "echo '$public_ip $private_dns_name' | sudo tee -a /etc/hosts"

echo "Adding $USER_AT_HOST to $ENV"
juju --show-log add-machine -e $ENV ssh:$USER_AT_HOST
