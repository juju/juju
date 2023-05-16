run_upgrade_charm_with_bind() {
	echo

	file="${TEST_DIR}/test-upgrade-charm-with-bind-ec2.log"

	ensure "spaces-upgrade-charm-with-bind-ec2" "${file}"

	## Setup spaces
	juju reload-spaces
	juju add-space isolated 172.31.254.0/24

	# Create machine
	# Note that due to the way that run_* funcs are executed, $1 holds the
	# test name so the NIC ID is actually provided in $2
	hotplug_nic_id=$2
	add_multi_nic_machine "$hotplug_nic_id"

	juju_machine_id=$(juju show-machine --format json | jq -r '.["machines"] | keys[0]')
	ifaces=$(juju ssh ${juju_machine_id} 'ip -j link' | jq -r '.[].ifname | select(. | startswith("en") or startswith("eth"))')
	primary_iface=$(echo $ifaces | cut -d " " -f1)
	hotplug_iface=$(echo $ifaces | cut -d " " -f2)
	configure_multi_nic_netplan "$juju_machine_id" "$hotplug_iface"

	# Deploy test charm to dual-nic machine
	juju deploy ./testcharms/charms/space-defender --bind "defend-a=alpha defend-b=isolated" --to "${juju_machine_id}"
	unit_index=$(get_unit_index "space-defender")
	wait_for "space-defender" "$(idle_condition "space-defender" 0 "${unit_index}")"

	assert_net_iface_for_endpoint_matches "space-defender" "defend-a" "${primary_iface}"
	assert_net_iface_for_endpoint_matches "space-defender" "defend-b" "${hotplug_iface}"

	assert_endpoint_binding_matches "space-defender" "" "alpha"
	assert_endpoint_binding_matches "space-defender" "defend-a" "alpha"
	assert_endpoint_binding_matches "space-defender" "defend-b" "isolated"

	# Upgrade the space-defender charm and modify its bindings
	juju refresh space-defender --bind "defend-a=alpha defend-b=alpha" --path ./testcharms/charms/space-defender
	wait_for "space-defender" "$(idle_condition "space-defender" 0 "${unit_index}")"

	# After the upgrade, defend-a should remain attached to ens5 but
	# defend-b which has now been bound to alpha should also get ens5
	assert_net_iface_for_endpoint_matches "space-defender" "defend-a" "${primary_iface}"
	assert_net_iface_for_endpoint_matches "space-defender" "defend-b" "${primary_iface}"

	assert_endpoint_binding_matches "space-defender" "" "alpha"
	assert_endpoint_binding_matches "space-defender" "defend-a" "alpha"
	assert_endpoint_binding_matches "space-defender" "defend-b" "alpha"

	destroy_model "spaces-upgrade-charm-with-bind-ec2"
}

test_upgrade_charm_with_bind() {
	if [ "$(skip 'test_upgrade_charm_with_bind')" ]; then
		echo "==> TEST SKIPPED: upgrade charm with --bind"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_upgrade_charm_with_bind" "$@"
	)
}
