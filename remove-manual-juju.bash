#!/bin/bash

ips="$@"
for ip in $ips; do
    ssh -i $JUJU_HOME/staging-juju-rsa ubuntu@$ip <<"EOT"
#!/bin/bash
set -ux
JUJU_DIR="/var/lib/juju"
DUMMY_DIR="/var/run/dummy-sink"
if [[ -n "$(ps -C jujud --no-headers)" ]]; then
    sudo touch $JUJU_DIR/uninstall-agent
    sudo killall -SIGABRT jujud
fi
sudo killall -9 mongod || true
if [[ -d $JUJU_DIR ]]; then
    sudo rm -r $JUJU_DIR
fi
if [[ -d $DUMMY_DIR ]]; then
    sudo rm -r $DUMMY_DIR
fi
sudo find /etc/init -name 'juju*' -delete
EOT
done
