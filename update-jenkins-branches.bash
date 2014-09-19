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
    set +e
    if [[ "$CLOUD_CITY" == "true" ]]; then
        set +e
        bzr branch lp:~juju-qa/+junk/cloud-city \
            bzr+ssh://jenkins@$host/var/lib/jenkins/cloud-city.new
        set -e
    fi
    ssh jenkins@$host << EOT
#!/bin/bash
set -eux
if [[ "$CLOUD_CITY" == "true" ]]; then
    (cd ~/cloud-city; bzr revert; cd -)
    bzr pull -d ~/cloud-city ~/cloud-city.new
    rm -r ~/cloud-city.new
fi
cd ~/juju-release-tools
bzr pull
cd ~/juju-ci-tools
bzr pull
if [[ -d ~/ci-director ]]; then
    cd ~/ci-director
    bzr pull
fi

sudo apt-get update || "! $host is not setup for sudo"
sudo apt-get install s3cmd python-requests

if [[ "$NEW_JUJU" == "true" ]]; then
    sudo apt-get install -y juju-local juju \
        uvtool-libvirt uvtool python-novaclient euca2ools || echo \
            "! Could not update juju on $host"
fi
EOT
}


CLOUD_CITY="false"
NEW_JUJU="false"
while [[ "${1-}" != "" ]]; do
    case $1 in
        --cloud-city)
            CLOUD_CITY="true"
            ;;
        --new-juju)
            NEW_JUJU="true"
            ;;
    esac
    shift
done

SLAVES=$(juju status '*-slave*' | grep public-address | sed -r 's,^.*: ,,')
if [[ -z $SLAVES ]]; then
    echo "Set JUJU_HOME to juju-qa's environments and switch to juju-ci."
    exit 1
fi

SKIPPED=""
for host in $MASTER $SLAVES; do
    update_jenkins $host || SKIPPED="$SKIPPED $host"
done

if [[ -n "$SKIPPED" ]]; then
    set +x
    echo
    echo "These hosts were skipped because thee was an error"
    echo "$SKIPPED"
fi

