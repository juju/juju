#!/bin/bash

JUJU_DIR="/var/lib/juju"
DUMMY_DIR="/var/run/dummy-sink"
ips="$@"
for ip in $ips; do
    ssh -i $JUJU_HOME/staging-juju-rsa ubuntu@$ip <<EOT
#!/bin/bash
set -ux
if [[ -n "$(ps ax | grep jujud | grep -v grep)" ]]; then
    sudo killall -SIGABRT jujud
fi
if [[ -d $JUJU_DIR ]]; then
    sudo rm -r $JUJU_DIR
fi
if [[ -d $DUMMY_DIR ]]; then
    sudo rm -r $DUMMY_DIR
fi
sudo find /etc/init -name 'juju*' -delete
EOT
done
