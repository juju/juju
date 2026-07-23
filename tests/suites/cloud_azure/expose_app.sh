run_expose_app_azure() {
	local app_name="ubuntu-lite"
	local all_ports="1337-1339/tcp"
	local endpoint="ubuntu"
	local endpoint_ports="1234/tcp"

	echo

	file="${TEST_DIR}/test-expose-app-azure.log"

	ensure "expose-app-azure" "${file}"

	# Deploy test charm with dual-stack constraint so the machine has both
	# an IPv4 and an IPv6 address for NSG rule verification.
	juju deploy "${app_name}" --constraints "ip-family=dual"
	wait_for "${app_name}" "$(idle_condition "${app_name}")"

	# Open ports and verify hook tool behavior (provider-agnostic).
	assert_opened_ports_output \
		"${app_name}" "${all_ports}" "${endpoint}" "${endpoint_ports}"

	# Ensure that CIDRs are correctly generated in Azure NSG rules.
	assert_ingress_cidrs_for_exposed_app_azure \
		"${app_name}" "${all_ports}" "${endpoint}" "${endpoint_ports}"

	# Verify IPv4 and IPv6 reachability through the exposed NSG.
	assert_dual_stack_reachability_for_exposed_app_azure "${app_name}" "${all_ports}"

	destroy_model "expose-app-azure"
}

assert_ingress_cidrs_for_exposed_app_azure() {
	local app_name all_ports endpoint endpoint_ports
	local bounds all_from all_to endpoint_from endpoint_to

	if [ "$#" -lt 2 ] || [ "$#" -gt 4 ]; then
		echo "ERROR: usage: assert_ingress_cidrs_for_exposed_app_azure <app> <all-ports> [<endpoint> <endpoint-ports>]" >&2
		return 1
	fi

	app_name=${1:-}
	all_ports=${2:-}
	endpoint=${3:-}
	endpoint_ports=${4:-}
	if [ -z "${app_name}" ] || [ -z "${all_ports}" ]; then
		echo "ERROR: application and all-endpoints port range are required" >&2
		return 1
	fi
	if { [ -n "${endpoint}" ] && [ -z "${endpoint_ports}" ]; } || \
		{ [ -z "${endpoint}" ] && [ -n "${endpoint_ports}" ]; }; then
		echo "ERROR: endpoint and endpoint port range must be supplied together" >&2
		return 1
	fi

	if ! bounds=$(azure_port_range_bounds "${all_ports}"); then
		return 1
	fi
	read -r all_from all_to <<< "${bounds}"
	if [ -n "${endpoint}" ]; then
		if ! bounds=$(azure_port_range_bounds "${endpoint_ports}"); then
			return 1
		fi
		read -r endpoint_from endpoint_to <<< "${bounds}"
	fi

	echo "==> Checking that expose --to-cidrs works as expected on Azure dual-stack"

	# IPv4-only expose: no IPv6 CIDR is specified anywhere, so no IPv6
	# source rule may be generated (core firewaller behavior).
	juju expose "${app_name}" --to-cidrs 10.0.0.0/24,192.168.0.0/24

	echo "==> Waiting for Azure NSG rules to be updated"
	wait_for_azure_nsg_ingress_cidrs_for_port_range \
		"${all_from}" "${all_to}" "10.0.0.0/24,192.168.0.0/24" "ipv4"

	# Regression guard, checked only after the IPv4 rules are confirmed
	# present: the firewaller has then processed the expose, so the
	# absence of IPv6 rules is meaningful. Let any trailing rule creation
	# settle first.
	sleep "${SHORT_TIMEOUT}"
	echo "==> Regression guard: no IPv6 NSG rules after IPv4-only expose"
	assert_no_ipv6_nsg_rules_for_port_range "${all_from}" "${all_to}"

	if [ -n "${endpoint}" ]; then
		# Verify that a subsequent expose overwrites the previous rule for
		# the same endpoint: first open to the world, then restrict to
		# specific CIDRs.
		juju expose "${app_name}" --endpoints "${endpoint}"
		juju expose "${app_name}" --endpoints "${endpoint}" \
			--to-cidrs 10.42.0.0/16,2002:0:0:1234::/64

		echo "==> Waiting for Azure NSG rules to be updated"
		# The all-endpoints range also receives the named endpoint CIDRs.
		wait_for_azure_nsg_ingress_cidrs_for_port_range \
			"${all_from}" "${all_to}" \
			"10.0.0.0/24,10.42.0.0/16,192.168.0.0/24" "ipv4"
		wait_for_azure_nsg_ingress_cidrs_for_port_range \
			"${all_from}" "${all_to}" \
			"2002:0:0:1234::/64" "ipv6"

		# The endpoint-specific range should only use its endpoint CIDRs.
		wait_for_azure_nsg_ingress_cidrs_for_port_range \
			"${endpoint_from}" "${endpoint_to}" "10.42.0.0/16" "ipv4"
		wait_for_azure_nsg_ingress_cidrs_for_port_range \
			"${endpoint_from}" "${endpoint_to}" \
			"2002:0:0:1234::/64" "ipv6"
	fi

	# Destination family check (juju/juju#22758): expose-created rules
	# must route each source CIDR to a destination of the same family.
	echo "==> Checking NSG rule destinations match source address family"
	assert_nsg_rule_destinations_match_source_family
}

# assert_no_ipv6_nsg_rules_for_port_range fails if any inbound Allow NSG
# rule for the given port range has an IPv6 source CIDR. It verifies that
# an IPv4-only expose does not generate IPv6 source rules.
assert_no_ipv6_nsg_rules_for_port_range() {
	local from_port="${1}" to_port="${2}"
	local port_range instance_id rg nsg_name got_v6

	port_range="${from_port}"
	if [ "${from_port}" != "${to_port}" ]; then
		port_range="${from_port}-${to_port}"
	fi

	read -r instance_id rg <<< "$(azure_first_model_instance)"
	nsg_name=$(azure_nsg_name_for_instance "${rg}" "${instance_id}")

	got_v6=$(az network nsg rule list --resource-group "${rg}" --nsg-name "${nsg_name}" -o yaml 2>/dev/null | \
		yq -r ".[] | select(.destinationPortRange == \"${port_range}\" and .access == \"Allow\" and .direction == \"Inbound\") | .sourceAddressPrefix | select(test(\":\"))" | sort | paste -sd, -)
	if [ -n "${got_v6}" ]; then
		# shellcheck disable=SC2046
		echo $(red "unexpected IPv6 NSG rule for port range [${from_port},${to_port}]: ${got_v6}")
		exit 1
	fi
}

# assert_nsg_rule_destinations_match_source_family fails if any of this
# machine's expose-created NSG rules (named machine-<id>-*) does not route
# its source CIDR to the instance's private address of the same family:
# the primary IPv4 address for IPv4 sources, the first IPv6 address for
# IPv6 sources (mirrors the destination selection of juju/juju#22758).
assert_nsg_rule_destinations_match_source_family() {
	local machine_id instance_id rg nsg_name nic_v4 nic_v6 rules_yaml bad_v4 bad_v6 mismatched

	machine_id=$(juju show-machine --format json | yq -r '.machines | keys | .[0]')
	read -r instance_id rg <<< "$(azure_first_model_instance)"
	nsg_name=$(azure_nsg_name_for_instance "${rg}" "${instance_id}")
	read -r nic_v4 nic_v6 <<< "$(azure_nic_private_addresses "${rg}" "${instance_id}")"

	rules_yaml=$(az network nsg rule list --resource-group "${rg}" --nsg-name "${nsg_name}" -o yaml 2>/dev/null)

	# Guard against a vacuous pass: if no expose-created rule matches the
	# machine prefix, there is nothing to validate, so fail loudly.
	matched=$(printf '%s\n' "${rules_yaml}" | MACHINE_ID="${machine_id}" yq -r \
		'[.[] | select(.access == "Allow" and .direction == "Inbound") |
			select(.name | test("^machine-" + strenv(MACHINE_ID) + "-"))] | length')
	if [ "${matched}" -eq 0 ]; then
		# shellcheck disable=SC2046
		echo $(red "no expose-created NSG rules found for machine ${machine_id}; expected machine-${machine_id}-*")
		exit 1
	fi

	bad_v4=$(printf '%s\n' "${rules_yaml}" | MACHINE_ID="${machine_id}" NIC="${nic_v4}" yq -r \
		'.[] | select(.access == "Allow" and .direction == "Inbound") |
			select(.name | test("^machine-" + strenv(MACHINE_ID) + "-")) |
			select((.sourceAddressPrefix // "") | test(":") | not) |
			select((.destinationAddressPrefix // "<none>") != strenv(NIC)) |
			[.name, .sourceAddressPrefix, (.destinationAddressPrefix // "<none>")] | join(" ")')
	bad_v6=$(printf '%s\n' "${rules_yaml}" | MACHINE_ID="${machine_id}" NIC="${nic_v6}" yq -r \
		'.[] | select(.access == "Allow" and .direction == "Inbound") |
			select(.name | test("^machine-" + strenv(MACHINE_ID) + "-")) |
			select((.sourceAddressPrefix // "") | test(":")) |
			select((.destinationAddressPrefix // "<none>") != strenv(NIC)) |
			[.name, .sourceAddressPrefix, (.destinationAddressPrefix // "<none>")] | join(" ")')
	mismatched=$(printf '%s\n%s\n' "${bad_v4}" "${bad_v6}" | sed '/^$/d' | sort)
	if [ -n "${mismatched}" ]; then
		# shellcheck disable=SC2046
		echo $(red "NSG rules not routed to the instance's private address of the matching family:\n${mismatched}")
		exit 1
	fi
}

assert_dual_stack_reachability_for_exposed_app_azure() {
	local app_name all_ports bounds port unit machine_id
	local unit_ipv4 unit_ipv6

	if [ "$#" -ne 2 ]; then
		echo "ERROR: usage: assert_dual_stack_reachability_for_exposed_app_azure <app> <all-ports>" >&2
		return 1
	fi

	app_name=${1:-}
	all_ports=${2:-}
	if [ -z "${app_name}" ] || [ -z "${all_ports}" ]; then
		echo "ERROR: application and all-endpoints port range are required" >&2
		return 1
	fi
	if ! bounds=$(azure_port_range_bounds "${all_ports}"); then
		return 1
	fi
	port=${bounds%% *}
	unit="${app_name}/0"

	echo "==> Checking IPv4 and IPv6 reachability through exposed NSG"

	# Expose the all-endpoints rule to the world so the CI runner can probe
	# the actual port opened above. CIDR behavior was verified separately.
	juju expose "${app_name}"
	wait_for "${app_name}" "$(idle_condition "${app_name}")"

	machine_id=$(juju show-unit "${unit}" --format json | UNIT="${unit}" yq -r '.[strenv(UNIT)].machine')
	if [ -z "${machine_id}" ] || [ "${machine_id}" = "null" ]; then
		echo "ERROR: no machine found for ${unit}"
		return 1
	fi
	unit_ipv6=$(machine_ipv6 "${machine_id}")
	unit_ipv4=$(juju show-machine "${machine_id}" --format json |
		MACHINE_ID="${machine_id}" yq -r '.machines[strenv(MACHINE_ID)]["ip-addresses"][] |
			select(. | test("^[0-9]+[.]"))' |
		head -1)

	if [ -z "${unit_ipv6}" ]; then
		echo "ERROR: no IPv6 address found on machine ${machine_id} (expected dual-stack)"
		return 1
	fi
	if [ -z "${unit_ipv4}" ]; then
		echo "ERROR: no IPv4 address found on machine ${machine_id}"
		return 1
	fi

	if ! ip -6 route show default | grep -q .; then
		echo "==> ERROR: no IPv6 egress on CI runner, cannot verify IPv6 reachability"
		return 1
	fi
	if ! juju exec --unit "${unit}" -- "command -v python3" >/dev/null 2>&1; then
		echo "ERROR: python3 is not available on ${unit}"
		return 1
	fi
	if ! azure_upload_tcp_listener "${unit}"; then
		azure_remove_tcp_listener "${unit}" || true
		echo "ERROR: could not upload the TCP listener to ${unit}"
		return 1
	fi

	# Probe one bounded, one-shot listener per address family. Each retry
	# starts a fresh listener so a transient connection cannot consume the
	# only probe while the firewaller propagates the expose rule.
	echo "==> Probing exposed port ${port} over IPv4 (${unit_ipv4})"
	if ! azure_probe_tcp_listener "${unit}" "ipv4" "${port}" "${unit_ipv4}"; then
		azure_remove_tcp_listener "${unit}" || true
		echo "ERROR: IPv4 connection to ${unit_ipv4}:${port} failed"
		return 1
	fi

	echo "==> Probing exposed port ${port} over IPv6 (${unit_ipv6})"
	if ! azure_probe_tcp_listener "${unit}" "ipv6" "${port}" "${unit_ipv6}"; then
		azure_remove_tcp_listener "${unit}" || true
		echo "ERROR: IPv6 connection to [${unit_ipv6}]:${port} failed"
		return 1
	fi

	azure_remove_tcp_listener "${unit}" || \
		echo "WARNING: could not remove the uploaded listener script on ${unit}"
}

test_expose_app_azure() {
	if [ "$(skip 'test_expose_app_azure')" ]; then
		echo "==> TEST SKIPPED: juju expose_app_azure"
		return
	fi

	if [ "${BOOTSTRAP_PROVIDER}" != "azure" ]; then
		echo "==> TEST SKIPPED: expose_app_azure tests, not using azure"
		return
	fi

	if [ "$(az account list | yq -r 'length')" -lt 1 ]; then
		echo "==> TEST SKIPPED: not logged in to Azure cloud"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_expose_app_azure" "$@"
	)
}
