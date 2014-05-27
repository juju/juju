#!/bin/bash
# As a member of juju-qa,  Visit each the jenkins master and slaves
# and update their branches.
# passing 'true' as an arg will driect the script to try to update cloud-city.
set -eux

MASTER="juju-ci.vapour.ws"
KEY="staging-juju-rsa"


update_jenkins() {
    host=$1
    echo "updating $host"
    if [[ "$CLOUD_CITY" == "true" ]]; then
        is_active=$(
            ssh jenkins@$host "find ~/cloud-city/environments/ -name *.jenv")
        if [[ -n "$is_active" ]]; then
            echo "$host has jenvs. Either clear the files or run with 'false'."
            exit 2
        fi
        ssh jenkins@$host "mv ~/cloud-city ~/cloud-city.old"
        bzr branch lp:~juju-qa/+junk/cloud-city \
            bzr+ssh://jenkins@$host/var/lib/jenkins/cloud-city
    fi
    ssh jenkins@$host << EOT
#!/bin/bash
set -eux
if [[ "$CLOUD_CITY" == "true" ]]; then
    bzr checkout ~/cloud-city ~/cloud-city
    chmod 600 ~/cloud-city/$KEY*
    sudo rm -r ~/cloud-city.old
fi
cd ~/juju-release-tools
bzr pull
cd ~/juju-ci-tools
bzr pull
EOT
}


CLOUD_CITY=${1:-false}
SLAVES=$(juju status *-slave | grep public-address | sed -r 's,^.*: ,,')
if [[ -z slaves ]]; then
    echo "Set JUJU_HOME to juju-qa's environments and switch to juju-ci."
    exit 1
fi

for host in $MASTER $SLAVES; do
    update_jenkins $host
done

