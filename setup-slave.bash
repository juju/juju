#!/bin/bash
# setup-slave.bash arm64-slave [./cloud-city] [jenkins-master-ip]
set -eux

SLAVE=$1
LOCAL_CLOUD_CITY="${2:-./cloud-city}"
MASTER="http://juju-ci.vapour.ws:8080/"
KEY="staging-juju-rsa"
SLAVE_ADDRESS=$(juju status $SLAVE |
    grep public-address |
    cut -d : -f 2 |
    sed 's/ //g')

# Copy the authorized_keys so that we can ssh as jenkins.
ssh -i $LOCAL_CLOUD_CITY/$KEY ubuntu@$SLAVE_ADDRESS <<EOT
echo 'jenkins ALL=(ALL) NOPASSWD:ALL' | sudo tee -a /etc/sudoers.d/91-jenkins
test -d /var/lib/jenkins/.ssh/ || sudo mkdir -p /var/lib/jenkins/.ssh/
cat ./.ssh/authorized_keys | sudo tee -a /var/lib/jenkins/.ssh/authorized_keys
sudo chmod 700 /var/lib/jenkins/.ssh/
sudo chmod 600 ./.ssh/authorized_keys
sudo chown -R jenkins:jenkins /var/lib/jenkins/.ssh
EOT

# Install ssh rules for juju to repeatedly create instances.
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$SLAVE_ADDRESS \
"cat << EOC | tee -a /var/lib/jenkins/.ssh/config
Host 10.*
  StrictHostKeyChecking no
  UserKnownHostsFile /dev/null
  User ubuntu
  IdentityFile /var/lib/jenkins/cloud-city/$KEY
EOC"

# Place a copy of cloud city on the slave as jenkins using your Lp privs.
bzr branch lp:~juju-qa/+junk/cloud-city \
    bzr+ssh://jenkins@$SLAVE_ADDRESS/var/lib/jenkins/cloud-city

# Realise the private branch, then get the other branches.
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$SLAVE_ADDRESS <<EOT
bzr checkout cloud-city cloud-city
bzr branch lp:juju-release-tools juju-release-tools
bzr branch lp:juju-ci-tools juju-ci-tools
bzr branch lp:juju-ci-tools/repository repository
chmod 600 cloud-city/$KEY*
ln -s $HOME/cloud-city/$KEY .ssh/id_rsa
ln -s $HOME/cloud-city/$KEY.pub .ssh/id_rsa.pub
EOT

# Install stable juju.
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$SLAVE_ADDRESS <<EOT
sudo apt-add-repository -y ppa:juju/stable
sudo apt-get update
sudo apt-get install -y juju-local juju uvtool-libvirt uvtool
sudo /usr/share/jenkins/bin/download-slave.sh $MASTER
EOT

# Configure Jenkins with launch command
echo "Set the slave's launch method to to use the ssh gateway."
