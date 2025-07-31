run_juju_bind() {
	echo

	file="${TEST_DIR}/test-juju-bind.log"

	ensure "spaces-juju-bind" "${file}"

	## Setup spaces
	juju reload-spaces
	juju add-space isolated 172.31.254.0/24

	# Create machine
	# Note that due to the way that run_* funcs are executed, $1 holds the
	# test name so the NIC ID is actually provided in $2
	hotplug_nic_id=$2
	add_multi_nic_machine "$hotplug_nic_id" "spaces-juju-bind"

	juju_machine_id=$(juju show-machine --format json | jq -r '.["machines"] | keys[0]')
	ifaces=$(juju ssh ${juju_machine_id} 'ip -j link' | jq -r '.[].ifname | select(. | startswith("en") or startswith("eth"))')
	primary_iface=$(echo $ifaces | cut -d " " -f1)
	hotplug_iface=$(echo $ifaces | cut -d " " -f2)
	configure_multi_nic_netplan "$juju_machine_id" "$hotplug_iface" "spaces-juju-bind"

	# Deploy test charm to dual-nic machine
	# shellcheck disable=SC2046
	juju deploy $(pack_charm ./testcharms/charms/space-defender) --bind "defend-a=alpha defend-b=isolated" --to "${juju_machine_id}"
	unit_index=$(get_unit_index "space-defender")
	wait_for "space-defender" "$(idle_condition "space-defender" "${unit_index}")"

	assert_net_iface_for_endpoint_matches "space-defender" "defend-a" "${primary_iface}"
	assert_net_iface_for_endpoint_matches "space-defender" "defend-b" "${hotplug_iface}"

	assert_endpoint_binding_matches "space-defender" "" "alpha"
	assert_endpoint_binding_matches "space-defender" "defend-a" "alpha"
	assert_endpoint_binding_matches "space-defender" "defend-b" "isolated"

	# Mutate bindings
	juju bind space-defender defend-a=alpha defend-b=alpha

	# After the upgrade, defend-a should remain attached to ens5 but
	# defend-b which has now been bound to alpha should also get ens5
	assert_net_iface_for_endpoint_matches "space-defender" "defend-a" "${primary_iface}"
	assert_net_iface_for_endpoint_matches "space-defender" "defend-b" "${primary_iface}"

	assert_endpoint_binding_matches "space-defender" "" "alpha"
	assert_endpoint_binding_matches "space-defender" "defend-a" "alpha"
	assert_endpoint_binding_matches "space-defender" "defend-b" "alpha"

	destroy_model "spaces-juju-bind"
}

test_juju_bind() {
	if [ "$(skip 'test_juju_bind')" ]; then
		echo "==> TEST SKIPPED: juju bind"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_juju_bind" "$@"
	)
}
