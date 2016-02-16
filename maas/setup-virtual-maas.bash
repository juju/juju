#!/bin/bash
set -eux
help () {
  echo "${0##*/} public_ip_address dns_ip [cloud-city] [maas-repo]
        public_ip_address: Public IP of the host system.
        dns_ip: DNS to which non-maas queries should be forwarded.
        cloud-city: Path to cloud-city.
        maas-repo: "next"
  "
        exit 1
  }

[[ $# < "2" ]] && help
PUBLIC_ADDRESS=$1
DNS_ADDRESS=$2
shift 2
[[ $# > "2" ]] && help
LOCAL_CLOUD_CITY="${1:-./cloud-city}"
MAAS_REPO="${2:-default}"
KEY="staging-juju-rsa"
PUB_KEY_VALUE=$(cat ${LOCAL_CLOUD_CITY}/${KEY}.pub)
WIN_CERT_KEY=$(cat ${LOCAL_CLOUD_CITY}/winrm_client_cert.pem)
MAAS_ADDRESS="http://${PUBLIC_ADDRESS}/MAAS"
MAAS_API_ADDRESS="${MAAS_ADDRESS}/api/1.0"
MAAS_HOME="/var/lib/maas"

# Install required packages
ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$PUBLIC_ADDRESS <<EOT
[[ -d \$HOME/juju-ci-tools/maas ]] || { echo "\$HOME/juju-ci-tools/maas is required."; exit 1; }

sudo apt-get update
sudo apt-get install -y libvirt-bin
sudo apt-get install -y cloud-utils
sudo apt-get install -y qemu-kvm
sudo apt-get install -y virt-manager
sudo usermod -G libvirtd jenkins
EOT

ssh -i $LOCAL_CLOUD_CITY/$KEY jenkins@$PUBLIC_ADDRESS <<EOT
[[ -d \$HOME/juju-ci-tools/maas ]] || { echo "\$HOME/juju-ci-tools/maas is required."; exit 1; }
# Setup libvirt network and pools
virsh net-define \$HOME/juju-ci-tools/maas/maasnet2.xml
virsh net-start maasnet2
virsh net-autostart maasnet2

virsh net-define \$HOME/juju-ci-tools/maas/maasprv1.xml
virsh net-start maasprv1
virsh net-autostart maasprv1

sudo mkdir -p /images/maaspool1
virsh pool-define \$HOME/juju-ci-tools/maas/maaspool1.xml
virsh pool-start maaspool1
virsh pool-autostart maaspool1

# Install MAAS
[[ $MAAS_REPO == "next" ]] && sudo add-apt-repository ppa:maas/next
sudo apt-get update
sudo apt-get install -y maas

# Configure the maas user to access virsh for KVM machine power management
sudo chsh -s /bin/bash maas
sudo mkdir -p ${MAAS_HOME}/.ssh
sudo cp -L \${HOME}/.ssh/id_rsa* ${MAAS_HOME}/.ssh
sudo chmod 400 ${MAAS_HOME}/.ssh/rsa_id
sudo chown -R maas:maas ${MAAS_HOME}/.ssh
sudo chmod 700 ${MAAS_HOME}/.ssh

# Configure MAAS
sudo maas-region-admin createsuperuser --noinput --username root --email ubuntu@localhost
APIKEY=\$(sudo maas-region-admin apikey --username=root)
maas login vmaas $MAAS_API_ADDRESS \$APIKEY
maas vmaas sshkeys new key="$PUB_KEY_VALUE"
maas vmaas sslkeys new key="$WIN_CERT_KEY"


# Select and import images
maas vmaas boot-source update 1 url="http://maas.ubuntu.com/images/ephemeral-v2/daily/"

# FIXME: Different version of MAAS behave differently when selecting images
#        and setting network interfaces. Punt for now and print a message with
#        the network values that should be manually set.
#maas vmaas boot-source-selections create 1 release="precise" arches="amd64" subarches="*" labels="*" os="ubuntu"
#maas vmaas boot-source-selections create 1 release="trusty" arches="amd64" subarches="*" labels="*" os="ubuntu"
#maas vmaas boot-source-selections create 1 release="wily" arches="amd64" subarches="*" labels="*" os="ubuntu"
#maas vmaas boot-source-selections create 1 release="xenial" arches="amd64" subarches="*" labels="*" os="ubuntu"
#maas vmaas boot-source-selections create 1 release="centos70" arches="amd64" subarches="*" labels="*" os="centos"
#maas vmaas boot-resources import

## Setup MAAS to manage nodes on the internal 10.0.200 network
maas vmaas maas set-config name=upstream_dns value=$DNS_ADDRESS
#NODE_GROUP_ID=\$(maas vmaas node-groups list | grep -e '"uuid":' | awk '{print \$2}' | tr -d \\")
#echo "NODE_GROUP_ID=\$NODE_GROUP_ID"
#cmd="maas vmaas node-group-interface update \"\$NODE_GROUP_ID\" maasnetbr2 ip_range_high=10.0.200.199 ip_range_low=10.0.200.150 static_ip_range_high=10.0.200.249 static_ip_range_low=10.0.200.200 broadcast_ip=10.0.200.255 router_ip=10.0.200.1 management=2 subnet_mask=255.255.255.0"
#cmd="maas vmaas node-group-interfaces new \$NODE_GROUP_ID name=maasnetbr2 interface=maasnetbr2 ip=10.0.200.1 ip_range_high=10.0.200.199 ip_range_low=10.0.200.150 static_ip_range_high=10.0.200.249 static_ip_range_low=10.0.200.200 broadcast_ip=10.0.200.255 router_ip=10.0.200.1 management=2 subnet_mask=255.255.255.0"
#echo \$cmd
#\$cmd

[[ -n \$(df -h | grep -e /images\$) ]] || echo 'WARNING: /images is not a mounted filesystem, virtual machine images could fill up the root filesystem.'
EOT

# Configure Jenkins with launch command
echo "Set the MAAS root password with \"sudo maas-region-admin changepassword root\" from the jenkins account on $PUBLIC_ADDRESS."
echo "Log into $MAAS_ADDRESS"
echo "Add and/or configure network settings for maasnetbr2 to:
    Name: maasnetbr2
    Interface: maasnetbr2
    Management: DHCP and DNS
    IP: 10.0.200.1
    Subnet Mask: 255.255.255.0
    Broadcast IP: 10.0.200.255
    Router IP: 10.0.200.1
    DHCP dynamic IP range low value: 10.0.200.150
    DHCP dynamic IP range high value: 10.0.200.199
    Static IP range low value: 10.0.200.200
    Static IP range high value: 10.0.200.249
"
