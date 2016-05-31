#!/bin/bash
# Update the required resources on the Jenkins master and slaves.
# the --cloud-city option will also update credential and configs.
set -eux

SCRIPTS=$(readlink -f $(dirname $0))
REPOSITORY=${JUJU_REPOSITORY:-$(dirname $SCRIPTS)/repository}

MASTER="juju-ci.vapour.ws"
SLAVES="precise-slave.vapour.ws trusty-slave.vapour.ws \
    wily-slave.vapour.ws xenial-slave.vapour.ws \
    ppc64el-slave.vapour.ws arm64-slave.vapour.ws \
    kvm-slave.vapour.ws jujuqa-stack-slave.internal \
    munna-maas-slave.vapour.ws  silcoon-maas-slave.vapour.ws \
    canonistack-slave.vapour.ws juju-core-slave.vapour.ws \
    cloud-health-slave.vapour.ws certification-slave.vapour.ws \
    charm-bundle-slave.vapour.ws osx-slave.vapour.ws \
    s390x-slave.vapour.ws"
WIN_SLAVES="win-slave.vapour.ws"
KEY="staging-juju-rsa"
export JUJU_ENV="juju-ci3"

update_jenkins() {
    # Get the ip address which will most likely match historic ssh rules.
    hostname=$1
    if [[ $hostname == $MASTER ]]; then
        # Bypass DNS which points to the apache front-end.
        host="54.86.142.177"
    elif [[ $hostname =~ .*[.]internal$ ]]; then
        host=$hostname # resolved by /etc/hosts
    else
        host=$(host -4 -t A $hostname 8.8.8.8 | tail -1 | cut -d ' ' -f4)
    fi
    echo "updating $hostname at $host"
    if [[ "$CLOUD_CITY" == "true" ]]; then
        bzr branch lp:~juju-qa/+junk/cloud-city \
            bzr+ssh://jenkins@$host/var/lib/jenkins/cloud-city.new
    fi
    ssh jenkins@$host << EOT
#!/bin/bash
export PATH=/usr/local/bin:\$HOME/Bin:\$PATH
set -eux
if [[ "$CLOUD_CITY" == "true" ]]; then
    (cd ~/cloud-city; bzr revert; cd -)
    bzr pull -d ~/cloud-city ~/cloud-city.new
    rm -r ~/cloud-city.new
    sudo chown -R jenkins ~/cloud-city
    chmod -R go-w ~/cloud-city
    chmod 700 ~/cloud-city
    chmod 700 ~/cloud-city/gnupg
    chmod 600 ~/cloud-city/staging-juju-rsa
fi

bzr pull -d ~/juju-release-tools
bzr pull -d ~/repository
bzr pull -d ~/juju-ci-tools
if [[ ! -e ~/workspace-runner ]]; then
    bzr branch http://bazaar.launchpad.net/~juju-qa/workspace-runner/trunk/\
      ~/workspace-runner
fi
bzr pull -d ~/workspace-runner
if [[ \$(uname) == "Linux" ]]; then
    cd ~/juju-ci-tools
    make install-deps
fi
if [[ -d ~/ci-director ]]; then
    bzr pull -d ~/ci-director
fi
EOT
}


CLOUD_CITY="false"
while [[ "${1-}" != "" ]]; do
    case $1 in
        --cloud-city)
            CLOUD_CITY="true"
            ;;
    esac
    shift
done

SKIPPED=""
for hostname in $MASTER $SLAVES; do
    update_jenkins $hostname || SKIPPED="$SKIPPED $hostname"
    if [[ $hostname == "xenial-slave.vapour.ws" ]]; then
        echo "Curtis removed juju-deployer package to test the branch."
        ssh $hostname sudo apt-get remove -y juju-deployer python-jujuclient
    fi
done
# win-slaves have a different user and directory layout tan POSIX hosts.
for hostname in $WIN_SLAVES; do
    zip -q -r repository.zip $REPOSITORY -x *.bzr*
    scp repository.zip Administrator@$hostname:/cygdrive/c/Users/Administrator/
    ssh Administrator@$hostname << EOT
bzr pull -d ./juju-release-tools
bzr pull -d ./juju-ci-tools
/cygdrive/c/progra~2/7-Zip/7z.exe x -y repository.zip
EOT
done

if [[ -n "$SKIPPED" ]]; then
    set +x
    echo
    echo "These hosts were skipped because there was an error"
    echo "$SKIPPED"
fi

