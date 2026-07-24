# Azure-specific helper functions used by the cloud_azure test suite.
# Sourced automatically by run_test via import_subdir_files.

# azure_resource_group_for_instance <instance_id> [<model_name>]
#
# Prints the resource group for the given Azure instance id, preferring a
# scoped az vm show lookup when the (model-)configured resource-group-name
# is known. Falls back to a subscription-wide az vm list search.
azure_resource_group_for_instance() {
	local instance_id=${1}
	local model_name=${2:-}
	local rg model_rg

	rg=""
	if [ -n "${model_name}" ]; then
		model_rg=$(juju model-config -m "${model_name}" resource-group-name 2>/dev/null || true)
	else
		model_rg=$(juju model-config resource-group-name 2>/dev/null || true)
	fi
	if [ -n "${model_rg}" ] && [ "${model_rg}" != "null" ]; then
		rg=$(az vm show --resource-group "${model_rg}" --name "${instance_id}" --query "resourceGroup" -o tsv 2>/dev/null || true)
	fi
	if [ -z "${rg}" ]; then
		rg=$(az vm list -o yaml 2>/dev/null | yq -r ".[] | select(.name == \"${instance_id}\") | .resourceGroup")
	fi

	echo "${rg}"
}

# azure_first_model_instance prints "<instance_id> <resource_group>" for
# the first machine of the current model, discovered via the az CLI. The
# result is cached for the lifetime of the test process.
azure_first_model_instance() {
	if [ -n "${AZURE_MODEL_INSTANCE_ID:-}" ] && [ -n "${AZURE_MODEL_RG:-}" ]; then
		echo "${AZURE_MODEL_INSTANCE_ID} ${AZURE_MODEL_RG}"
		return 0
	fi

	local machine_id instance_id rg

	machine_id=$(juju show-machine --format json | yq -r '.machines | keys | .[0]')
	instance_id=$(juju show-machine "${machine_id}" --format json | yq -r ".machines[\"${machine_id}\"][\"instance-id\"]")
	rg=$(azure_resource_group_for_instance "${instance_id}")

	if [ -z "${rg}" ]; then
		echo "ERROR: could not find resource group for instance ${instance_id}" >&2
		return 1
	fi

	AZURE_MODEL_INSTANCE_ID="${instance_id}"
	AZURE_MODEL_RG="${rg}"
	export AZURE_MODEL_INSTANCE_ID AZURE_MODEL_RG
	echo "${instance_id} ${rg}"
}

# azure_nic_name_for_instance <resource_group> <instance_id>
# Prints the name of the given instance's primary NIC.
# On failure writes a diagnostic to stderr and returns non-zero.
azure_nic_name_for_instance() {
	local rg="${1}" instance_id="${2}"
	local nic_name

	nic_name=$(az network nic list --resource-group "${rg}" -o yaml 2>/dev/null | \
		yq -r ".[] | select(.virtualMachine.id | downcase | contains(\"${instance_id}\")) | .name" | head -1)
	if [ -z "${nic_name}" ]; then
		echo "ERROR: no NIC found in resource group ${rg} for instance ${instance_id}" >&2
		return 1
	fi
	echo "${nic_name}"
}

# azure_nic_private_addresses <resource_group> <instance_id>
# Prints "<ipv4> <ipv6>" for the instance's primary NIC, mirroring the
# azure provider's destination selection: the IPv4 address of the primary
# IP configuration, and the address of the first IPv6 configuration
# (empty when the machine is not dual-stack).
azure_nic_private_addresses() {
	local rg="${1}" instance_id="${2}"
	local nic_name nic_yaml v4 v6

	nic_name=$(azure_nic_name_for_instance "${rg}" "${instance_id}")
	nic_yaml=$(az network nic show --resource-group "${rg}" --name "${nic_name}" -o yaml 2>/dev/null)
	# Mirror the provider's destination selection: IPv4 from the primary
	# IP configuration, IPv6 from the first IPv6 configuration.
	v4=$(printf '%s\n' "${nic_yaml}" | yq -r '.ipConfigurations[] | select(.primary == true) | .privateIPAddress' | head -1)
	v6=$(printf '%s\n' "${nic_yaml}" | yq -r '.ipConfigurations[].privateIPAddress | select(test(":"))' | head -1)
	echo "${v4} ${v6}"
}

# azure_nsg_name_for_instance <resource_group> <instance_id>
# Discovers the Azure NSG attached to the given instance's primary NIC.
# Prefers the NIC-level NSG attachment, falling back to the NIC's subnet
# when the NIC itself has no NSG (juju 4.x azure provider attaches
# juju-internal-nsg at the subnet level rather than the NIC).
# Prints the NSG name on stdout; on failure writes a diagnostic to stderr
# and returns non-zero so the caller aborts loudly rather than silently.
azure_nsg_name_for_instance() {
	local rg="${1}" instance_id="${2}"
	local nic_name nsg_id subnet_id

	nic_name=$(azure_nic_name_for_instance "${rg}" "${instance_id}")
	nsg_id=$(az network nic show --resource-group "${rg}" --name "${nic_name}" -o yaml 2>/dev/null | \
		yq -r '.networkSecurityGroup.id // ""')
	if [ -z "${nsg_id}" ]; then
		subnet_id=$(az network nic show --resource-group "${rg}" --name "${nic_name}" -o yaml 2>/dev/null | \
			yq -r '.ipConfigurations[] | select(.primary == true) | .subnet.id // ""' | head -1)
		nsg_id=$(az network vnet subnet show --ids "${subnet_id}" -o yaml 2>/dev/null | \
			yq -r '.networkSecurityGroup.id // ""')
	fi
	if [ -z "${nsg_id}" ]; then
		echo "ERROR: no NSG found on NIC ${nic_name} or its subnet in resource group ${rg}" >&2
		return 1
	fi
	echo "${nsg_id}" | yq -r 'split("/") | .[-1]'
}

# wait_for_azure_nsg_ingress_cidrs_for_port_range blocks until the expected
# CIDRs are present in the Azure NSG rules for the specified port range and
# address family (ipv4 or ipv6). It discovers the resource group and NSG
# from the first machine in the current model via the az CLI.
#
# ```
# wait_for_azure_nsg_ingress_cidrs_for_port_range <from_port> <to_port> \
#     <expected_cidrs_csv> <cidr_type:ipv4|ipv6>
# ```
wait_for_azure_nsg_ingress_cidrs_for_port_range() {
	local from_port to_port exp_cidrs cidr_type instance_id rg nsg_name port_range fam_re got_cidrs attempt

	from_port=${1}
	to_port=${2}
	exp_cidrs=${3}
	cidr_type=${4}

	# Normalize the expected CIDR order to match the sorted output from az.
	exp_cidrs=$(printf '%s\n' "${exp_cidrs}" | tr ',' '\n' | sort | paste -sd, -)
	read -r instance_id rg <<< "$(azure_first_model_instance)"
	nsg_name=$(azure_nsg_name_for_instance "${rg}" "${instance_id}")

	port_range="${from_port}"
	if [ "${from_port}" != "${to_port}" ]; then
		port_range="${from_port}-${to_port}"
	fi

	fam_re="[.]"
	if [ "${cidr_type}" = "ipv6" ]; then
		fam_re=":"
	fi

	attempt=0
	while [ "${attempt}" -lt 3 ]; do
		echo "[+] (attempt ${attempt}) polling Azure NSG rules for ${port_range} ${cidr_type}"
		got_cidrs=$(az network nsg rule list --resource-group "${rg}" --nsg-name "${nsg_name}" -o yaml 2>/dev/null | \
			yq -r ".[] | select(.destinationPortRange == \"${port_range}\" and .access == \"Allow\" and .direction == \"Inbound\") | .sourceAddressPrefix | select(test(\"${fam_re}\"))" | sort | paste -sd, -)
		if [ "${got_cidrs}" = "${exp_cidrs}" ]; then
			break
		fi
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	if [ "${got_cidrs}" != "${exp_cidrs}" ]; then
		# shellcheck disable=SC2046
		echo $(red "expected Azure NSG ${cidr_type} CIDRs for range [${from_port}, ${to_port}] to be:\n${exp_cidrs}\nGOT:\n${got_cidrs}")
		exit 1
	fi

	echo "[+] NSG rules for port range [${from_port}, ${to_port}] and CIDRs ${exp_cidrs} updated"
}

# azure_upload_tcp_listener installs a one-shot HTTP listener script on a
# unit. The listener binds explicitly to one address family and writes a
# marker after bind/listen succeeds.
azure_upload_tcp_listener() {
	local unit=${1}
	local listener_script_b64

	listener_script_b64=$(base64 --wrap=0 <<'PYEOF'
import socket
import sys

family = sys.argv[1]
port = int(sys.argv[2])
ready_path = sys.argv[3]

if family == "ipv4":
    address_family = socket.AF_INET
    bind_address = "0.0.0.0"
elif family == "ipv6":
    address_family = socket.AF_INET6
    bind_address = "::"
else:
    raise ValueError(f"unsupported address family: {family}")

with socket.socket(address_family, socket.SOCK_STREAM) as server:
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    if address_family == socket.AF_INET6:
        server.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
    server.bind((bind_address, port))
    server.listen(1)
    with open(ready_path, "w", encoding="ascii"):
        pass
    connection, _ = server.accept()
    with connection:
        connection.recv(4096)
        connection.sendall(
            b"HTTP/1.1 200 OK\r\n"
            b"Content-Length: 2\r\n"
            b"Connection: close\r\n"
            b"\r\n"
            b"ok"
        )
PYEOF
	)

	juju exec --unit "${unit}" -- \
		"printf '%s' '${listener_script_b64}' | base64 -d > /tmp/juju-expose-app-listener.py && chmod 700 /tmp/juju-expose-app-listener.py"
}

# azure_start_tcp_listener starts the bounded listener for one address family.
azure_start_tcp_listener() {
	local unit=${1} family=${2} port=${3}
	local ready_file pid_file

	ready_file="/tmp/juju-expose-app-listener-${family}-${port}.ready"
	pid_file="/tmp/juju-expose-app-listener-${family}-${port}.pid"

	juju exec --unit "${unit}" -- \
		"rm -f ${ready_file} ${pid_file}; nohup timeout 30 python3 /tmp/juju-expose-app-listener.py ${family} ${port} ${ready_file} </dev/null >/dev/null 2>&1 & echo \$! > ${pid_file}"
}

# azure_wait_for_tcp_listener waits for the listener's bind marker without
# opening a connection that would consume its one-shot accept.
azure_wait_for_tcp_listener() {
	local unit=${1} family=${2} port=${3}
	local ready_file attempt

	ready_file="/tmp/juju-expose-app-listener-${family}-${port}.ready"
	attempt=0
	while [ "${attempt}" -lt 10 ]; do
		if juju exec --unit "${unit}" -- "test -f ${ready_file}" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
		attempt=$((attempt + 1))
	done
	return 1
}

# azure_stop_tcp_listener terminates the bounded listener and removes its
# marker and PID files without matching unrelated processes by name.
azure_stop_tcp_listener() {
	local unit=${1} family=${2} port=${3}
	local ready_file pid_file

	ready_file="/tmp/juju-expose-app-listener-${family}-${port}.ready"
	pid_file="/tmp/juju-expose-app-listener-${family}-${port}.pid"

	juju exec --unit "${unit}" -- \
		"if [ -r ${pid_file} ]; then kill \$(cat ${pid_file}) 2>/dev/null || true; fi; rm -f ${pid_file} ${ready_file}"
}

# azure_probe_tcp_listener retries a fresh one-shot listener while the
# firewaller propagates the final expose rule.
azure_probe_tcp_listener() {
	local unit=${1} family=${2} port=${3} address=${4}
	local url curl_family probe_out attempt max_attempts

	case ${family} in
	ipv4)
		url="http://${address}:${port}/"
		curl_family="--ipv4"
		;;
	ipv6)
		url="http://[${address}]:${port}/"
		curl_family="--ipv6"
		;;
	*)
		echo "ERROR: unsupported address family: ${family}" >&2
		return 1
		;;
	esac

	attempt=0
	max_attempts=6
	while [ "${attempt}" -lt "${max_attempts}" ]; do
		if azure_start_tcp_listener "${unit}" "${family}" "${port}" && \
			azure_wait_for_tcp_listener "${unit}" "${family}" "${port}"; then
			if probe_out=$(curl --fail --silent --show-error --noproxy '*' \
				"${curl_family}" --max-time 5 "${url}" 2>&1); then
				if printf '%s\n' "${probe_out}" | grep -q "ok"; then
					azure_stop_tcp_listener "${unit}" "${family}" "${port}" || true
					return 0
				fi
			fi
		fi
		azure_stop_tcp_listener "${unit}" "${family}" "${port}" || true
		attempt=$((attempt + 1))
		if [ "${attempt}" -lt "${max_attempts}" ]; then
			sleep 1
		fi
	done
	return 1
}

# azure_remove_tcp_listener removes the uploaded listener script.
azure_remove_tcp_listener() {
	local unit=${1}

	juju exec --unit "${unit}" -- \
		'rm -f /tmp/juju-expose-app-listener.py'
}
