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
set -euo pipefail
conf=/var/lib/juju/agents/machine-*/agent.conf

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

temp_dir=/tmp
if [[ "${client}" == "/snap/bin/juju-db.mongo" ]]; then
	temp_dir=/var/snap/juju-db/common
fi
user_file=""
password_file=""
cleanup() {
	if [[ -n "${user_file}" ]] && sudo test -f "${user_file}"; then
		sudo truncate -s 0 "${user_file}" || true
		sudo rm -f "${user_file}" || true
	fi
	if [[ -n "${password_file}" ]] && sudo test -f "${password_file}"; then
		sudo truncate -s 0 "${password_file}" || true
		sudo rm -f "${password_file}" || true
	fi
}
trap '{ set +ex; } 2>/dev/null; cleanup' EXIT

user_file=$(sudo mktemp -p "${temp_dir}" mongo-user.XXXXXX)
password_file=$(sudo mktemp -p "${temp_dir}" mongo-password.XXXXXX)

sudo awk '/^tag:/ {print $2}' ${conf} | sudo tee "${user_file}" >/dev/null
sudo awk '/^statepassword:/ {print $2}' ${conf} | sudo tee "${password_file}" >/dev/null
sudo chown root:root "${user_file}" "${password_file}"
sudo chmod 600 "${user_file}" "${password_file}"
if ! sudo test -s "${user_file}"; then
	echo "tag not found in ${conf}"
	exit 2
fi
if ! sudo test -s "${password_file}"; then
	echo "statepassword not found in ${conf}"
	exit 2
fi
mongo_auth_js="if (!db.getSiblingDB(\"admin\").auth(cat(\"${user_file}\").trim(), cat(\"${password_file}\").trim())) { print(\"mongo authentication failed\"); quit(3); }"

set -x
sudo ${client} localhost:37017/juju \
	--authenticationDatabase admin \
	--tls \
	${certs} \
	--eval "${mongo_auth_js}" \
	--shell
EOF

${juju} ssh --model controller ${machine} "${cmds}"
