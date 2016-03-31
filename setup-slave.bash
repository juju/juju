#!/bin/bash
# setup-slave.bash private_ip [public_address] [./cloud-city]
set -eux

MASTER="http://juju-ci.vapour.ws:8080/"
KEY="staging-juju-rsa"

SLAVE=$1
SLAVE_ADDRESS="${2:-$(juju status $SLAVE |
    grep public-address |
    cut -d : -f 2 |
    sed 's/ //g')}"
LOCAL_CLOUD_CITY="${3:-./cloud-city}"

SERIES=$(ssh -i $LOCAL_CLOUD_CITY/$KEY ubuntu@$SLAVE_ADDRESS "lsb_release -sc")
DEB_LINE="http://archive.ubuntu.com/ubuntu/ $SERIES-proposed restricted main multiverse universe"

# Put jenkins and lxc on large volume if possible.
ssh -i $LOCAL_CLOUD_CITY/$KEY ubuntu@$SLAVE_ADDRESS <<EOT
sudo apt-get install -y lxc
if [[ -n \$(df -h | grep /mnt | tr -s ' ' |  cut -d ' ' -f 4 | grep G) ]]; then
    if [[ ! -d /mnt/jenkins ]]; then
        sudo cp -rp /var/lib/jenkins /mnt
        sudo mv /var/lib/jenkins /var/lib/jenkins.old
        sudo ln -s /mnt/jenkins /var/lib/jenkins
    fi
    if [[ ! -d /mnt/lxc ]]; then
        sudo cp -rp /var/lib/lxc /mnt
        sudo mv /var/lib/lxc /var/lib/lxc.old
        sudo ln -s /mnt/lxc /var/lib/lxc
    fi
    if [[ ! -d /mnt/lxc && -d /var/lib/lxd ]]; then
        sudo cp -rp /var/lib/lxd /mnt
        sudo mv /var/lib/lxd /var/lib/lxd.old
        sudo ln -s /mnt/lxd /var/lib/lxd
    fi
fi
EOT

# Copy the authorized_keys so that we can ssh as jenkins.
ssh -i $LOCAL_CLOUD_CITY/$KEY ubuntu@$SLAVE_ADDRESS <<EOT
sudo sed -i -r "s,(127.0.0.1.*localhost),\1 $SLAVE," /etc/hosts
echo 'jenkins ALL=(ALL) NOPASSWD:ALL' | sudo tee -a /etc/sudoers.d/91-jenkins
sudo chmod 0440 /etc/sudoers.d/91-jenkins
test -d /var/lib/jenkins/.ssh/ || sudo mkdir -p /var/lib/jenkins/.ssh/
cat ./.ssh/authorized_keys | sudo tee -a /var/lib/jenkins/.ssh/authorized_keys
sudo chmod 700 /var/lib/jenkins/.ssh/
sudo chmod 600 ./.ssh/authorized_keys
sudo chown -R jenkins:jenkins /var/lib/jenkins/.ssh
EOT

# Install ssh rules for juju to repeatedly create instances.
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$SLAVE_ADDRESS \
"cat << EOC | tee -a /var/lib/jenkins/.ssh/config
Host 10.* 192.168.*
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  User ubuntu
  IdentityFile /var/lib/jenkins/cloud-city/$KEY
EOC
"

# Setup proposed.
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$SLAVE_ADDRESS \
"cat << EOC | sudo tee -a /etc/apt/preferences.d/proposed-updates
Package: *
Pin: release a=$SERIES-proposed
Pin-Priority: 400
EOC"

# Install stable juju.
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$SLAVE_ADDRESS <<EOT
echo "deb $DEB_LINE" | sudo tee -a /etc/apt/sources.list
sudo apt-add-repository -y ppa:juju/stable
sudo apt-get update
sudo apt-get install -y bzr make ntp
EOT

# Place a copy of cloud city on the slave as jenkins using your Lp privs.
bzr branch lp:~juju-qa/+junk/cloud-city \
    bzr+ssh://jenkins@$SLAVE_ADDRESS/var/lib/jenkins/cloud-city

# Realise the private branch, then get the other branches.
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$SLAVE_ADDRESS <<EOT
bzr checkout cloud-city cloud-city
bzr branch lp:workspace-runner workspace-runner
bzr branch lp:juju-release-tools juju-release-tools
bzr branch lp:juju-ci-tools juju-ci-tools
bzr branch lp:juju-ci-tools/repository repository
chmod 600 cloud-city/$KEY*
ln -s /var/lib/jenkins/cloud-city/$KEY .ssh/id_rsa
ln -s /var/lib/jenkins/cloud-city/$KEY.pub .ssh/id_rsa.pub
sudo /usr/share/jenkins/bin/download-slave.sh $MASTER
if [[ \$(uname) == "Linux" ]]; then
    cd ~/juju-ci-tools
    make install-deps
    cd ~/workspace-runner
    make install
fi
EOT

# Configure Jenkins with launch command
echo "Set the slave's launch method to to use the ssh gateway."
