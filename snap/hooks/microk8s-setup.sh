#!/usr/bin/env bash

set -eu

# maybe announce which trigger this is
env

if ! which microk8s.kubectl; then
    echo "microk8s is not installed."
    exit 0
fi

# get this version of juju
juju_version=$(/snap/bin/juju version | rev | cut -d- -f3- | rev)
docker_image=jujusolutions/jujud-operator:$juju_version
mongo_image=""

echo "Going to cache images: ${docker_image} and $mongo_image"

# Do I need sudo here? do I even have the right perms for this?
echo "Pulling: ${docker_image}"
microk8s.docker image pull ${docker_image}

echo "Pulling: ${mongo_image}"
microk8s.docker image pull ${mongo_image}

echo "Available images:"
microk8s.docker image ls

echo "Pre-loading microk8s cloud"
microk8s.config | /snap/bin/juju add-k8s microK8s
