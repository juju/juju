#!/bin/bash

EXITCODE=0

ips="$@"
for ip in $ips; do
    ssh -i $JUJU_HOME/staging-juju-rsa ubuntu@$ip <<"EOT"
#!/bin/bash
set -ux

DIRTY=0
JUJU_DIR="/var/lib/juju"
DUMMY_DIR="/var/run/dummy-sink"

echo "Cleaning manual machine"

# This is left by the test.
if [[ -d $DUMMY_DIR ]]; then
    sudo rm -r $DUMMY_DIR
fi

# Juju always leaves logs behind for debuging, and they are already collected.
sudo rm /var/log/juju/*.log || true

# Juju must cleanup these procs.
if ps -f -C jujud; then
    DIRTY=1
    echo "ERROR manual-provider: jujud left running."
    sudo touch $JUJU_DIR/uninstall-agent
    sudo killall -SIGABRT jujud
fi
if ps -f -C mongod; then
    DIRTY=1
    echo "ERROR manual-provider: mongod left running."
    sudo killall -9 mongod || true
fi
if [[ -d /etc/systemd/system ]]; then
    found=$(ls /etc/systemd/system/juju*)
    if [[ -n $found ]]; then
        DIRTY=1
        echo "ERROR manual-provider: systemd services left behind."
        for service_path in $found; do
            service=$(basename $service_path)
            sudo systemctl stop --force $service || true
            sudo systemctl disable $service || true
            sudo rm $service_path || true
        done
    fi
fi
if [[ -d /etc/init ]]; then
    found=$(find /etc/init -name 'juju*' -print)
    if [[ -n $found ]]; then
        DIRTY=1
        echo "ERROR manual-provider: upstart services left behind."
        sudo find /etc/init -name 'juju*' -delete || true
    fi
fi
if [[ -d $JUJU_DIR ]]; then
    DIRTY=1
    echo "ERROR manual-provider: $JUJU_DIR left behind."
    sudo rm -r $JUJU_DIR
fi
echo "Cleaning completed"
exit $DIRTY
EOT
    if [[ $? != 0 ]];then
        EXITCODE=1
    fi
done

exit $EXITCODE
