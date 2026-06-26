#!/bin/bash

set -u

declare -a cmds=()
declare -i machine=0

: ${juju:=$(command -v juju)}

while (($# > 0)); do
	case $1 in
	-h | --help)
		cat <<EOF
$(basename $0)

-h | --help         This help
-m | --machine N    Connect with machine N in the controller model
                    (defaults to ${machine})
EOF
		exit 0
		;;
	-m | --machine)
		shift
		machine=$(($1))
		;;
	*)
		echo "Unknown argument $1"
		exit 1
		;;
	esac
	shift
done

if [[ -v SNAP ]] && [[ -n ${SNAP} ]]; then
	juju=${SNAP}/bin/juju
fi

read -d '' -r cmds <<'EOF'
set -x
conf=/var/lib/juju/agents/machine-*/agent.conf
user=$(sudo awk '/^tag:/ {print $2}' ${conf})
password=$(sudo awk '/^statepassword:/ {print $2}' ${conf})

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

exec sudo ${client} localhost:37017/juju \
    --authenticationDatabase admin \
    --tls \
    ${certs} \
    --username "${user}" --password "${password}"
EOF

${juju} ssh --model controller ${machine} "${cmds}"
