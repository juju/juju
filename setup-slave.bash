#!/bin/bash
set -eux

SLAVE=$1
MASTER_ADDRESS=${2:-162.213.35.54}
MASTER="http://juju-ci.vapour.ws:8080/"
LOCAL_CLOUD_CITY="~/Work/cloud-city"
KEY="staging-juju-rsa"
SLAVE_ADDRESS=$(juju status ppc-slave |
    grep public-address |
    cut -d : -f 2 |
    sed 's/ //g')

# Register the slave with the master.
# This should be a charm responsability.
juju ssh $SLAVE/0 <<EOT
cd /usr/share/jenkins/bin
test -d /var/run/jenkins || sudo mkdir /var/run/jenkins
sudo ./download-slave.sh $MASTER
logout
EOT

# Copy the authorized_keys so that we can ssh as jenkins.
juju ssh $SLAVE/0 <<EOT
set -eux
cp /home/ubuntu/.ssh/authorized_keys /home/ubuntu/authorized_keys
sudo chown jenkins:jenkins /home/ubuntu/authorized_keys
sudo mv /home/ubuntu/authorized_keys /var/lib/jenkins/
sudo su jenkins
cd /var/lib/jenkins/
test -d ./.ssh/ || mkdir -p ./.ssh/
chmod 700 ./.ssh/
cat ./authorized_keys >> ./.ssh/authorized_keys
chmod 600 ./.ssh/authorized_keys
rm ./authorized_keys
exit
EOT

# Install ssh rules for juju to repeatedly create instances.
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$SLAVE_ADDRESS \
"cat << EOC | tee -a /var/lib/jenkins/.ssh/config
Host 10.55.*
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
ssh ppc-slave/0 -l jenkins
bzr checkout cloud-city cloud-city
bzr branch lp:juju-release-tools juju-release-tools
bzr branch lp:juju-ci-tools juju-ci-tools
bzr branch lp:juju-ci-tools/repository repository
chmod 600 cloud-city/$KEY*
ln -s cloud-city/$KEY .ssh/id_rsa
ln -s cloud-city/$KEY.pub .ssh/id_rsa.pub
ln -s juju-ci-tools ci-cd-scripts2
EOT

# Install stable juju.
juju ssh $SLAVE/0 <<EOT
sudo apt-add-repository -y ppa:juju/stable
sudo apt-get update
sudo apt-get install -y juju-local juju
EOT

# Configure Jenkins with launch command
echo "Set the slave's launch method to to use the ssh gateway.
