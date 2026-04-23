#!/bin/bash

set -x

conf=/var/lib/juju/agents/machine-*/agent.conf
user=$(sudo awk '/tag/ {print $2}' ${conf})
password=$(sudo awk '/statepassword/ {print $2}' ${conf})

if [[ -f /snap/bin/juju-db.mongo ]]; then
    client=/snap/bin/juju-db.mongo
elif [[ -f /usr/lib/juju/mongo*/bin/mongo ]]; then
    client=/usr/lib/juju/mongo*/bin/mongo
else
    client=/usr/bin/mongo
fi

certs=--tlsAllowInvalidCertificates
if sudo test -f /var/snap/juju-db/common/ca.crt; then
    certs="--tlsCertificateKeyFile=/var/snap/juju-db/common/server.pem --tlsCAFile=/var/snap/juju-db/common/ca.crt"
fi

sudo ${client} localhost:37017/juju \
    --authenticationDatabase admin \
    --tls \
    ${certs} \
    --username "${user}" --password "${password}"
