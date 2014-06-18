#!/bin/bash

JUJU_DIR="/var/lib/juju"
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
sudo find /etc/init -name 'juju*' -delete
EOT
done
