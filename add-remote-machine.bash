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

# add-remote-machine.bash juju-ci3 ./staging-juju-rsa ubuntu@10.245.67.134
env=$1
ssh_key=$2
user_at_host=$3

# Learn the state-server's public IP and private internl name DNS name.
private_dns_name=$(juju ssh 0 'echo "$(hostname).$(dnsdomainname)"')
public_dns_name=$(juju status | sed -r '/dns-name/!d; 1,1!d; s/.*: (.*)/\1/')
public_ip=$(dig "$public_dns_name" | sed -r '/^;/d; /IN/!d; s/.*A (.*)/\1/;')

echo "State-server public address: $public_ip $public_dns_name"
echo "State-server private address: $private_dns_name"

# Add the state-server internal ip and address to the gateway's /etc/hosts
echo "Adding state-server's private dns name to $user_at_host:/etc/hosts"
ssh -i $ssh_key $user_at_host \
    "echo '$public_ip $private_dns_name' | sudo tee -a /etc/hosts"

# Add the machine to juju-ci using the private address
echo "Adding $user_at_host to $env"
juju --show-log add-machine ssh:$user_at_host
