#!/bin/bash
# Commands to be run on a juju controller apiserver to enable system
# monitoring.

set +ex

collectd_config_file=$1
runner_script_output_path=$2

cat <<EOF > ${runner_script_output_path}
password=`sudo grep oldpassword /var/lib/juju/agents/machine-*/agent.conf | cut -d" " -f2`
mongostat --host=127.0.0.1:37017 \
    --authenticationDatabase admin \
    --ssl \
    --sslAllowInvalidCertificates \
    --username \"admin\" \
    --password \"\$password\" \
    --noheaders 5 > /tmp/mongodb-stats.log 2> /tmp/mongodb-error.log
EOF

sudo apt-get install -y collectd-core
sudo mkdir /etc/collectd/collectd.conf.d
sudo cp ${collectd_config_file} /etc/collectd/collectd.conf
sudo /etc/init.d/collectd restart

sudo echo "deb http://repo.mongodb.org/apt/ubuntu xenial/mongodb-org/testing multiverse" | sudo tee /etc/apt/sources.list.d/mongo-org.list
sudo apt-get update
sudo apt-get install --yes --allow-unauthenticated mongodb-org-tools=3.2.10~rc2 daemon
sudo chmod +x ${runner_script_output_path}
