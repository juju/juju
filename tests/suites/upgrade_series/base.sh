# Deploy the dummy-sink charm on an earlier base, then check that we can
# upgrade the base, and that the charm still behaves correctly.
run_upgrade_base_relation() {
	local start_base end_base
	start_base="ubuntu@20.04"
	end_base="ubuntu@22.04"

	# Setup
	ensure "test-upgrade-machine-relation" "${TEST_DIR}/test-upgrade-machine-relation.log"
	juju deploy ./testcharms/charms/dummy-sink --base $start_base
	juju deploy ./testcharms/charms/dummy-source --base $end_base
	juju relate dummy-sink dummy-source
	juju config dummy-source token=Canonical

	# Check pre-conditions
	wait_for "Canonical" "$(workload_status 'dummy-sink' 0).message"
	assert_machine_base 0 $start_base

	# Upgrade the machine
	juju upgrade-machine 0 prepare $end_base -y
	reboot_machine 0
	echo "Upgrading machine..."
	echo "See ${TEST_DIR}/do-release-upgrade.log for progress."
	juju ssh 0 'sudo do-release-upgrade -f DistUpgradeViewNonInteractive' &>"${TEST_DIR}/do-release-upgrade.log" || true
	reboot_machine 0
	juju upgrade-machine 0 complete

	# Check post-conditions
	wait_for "Canonical" "$(workload_status 'dummy-sink' 0).message"
	assert_machine_base 0 $end_base
	# Check post-series-upgrade hook has run
	#	juju show-status-log dummy-sink/0 | grep 'post-series-upgrade'
}

# Assert the given machine has the given series.
assert_machine_base() {
	local machine expected_base actual_base
	machine=$1
	expected_base=$2
	actual_base=$(juju status --format=json | jq -r ".machines[\"$machine\"] | (.base.name+\"@\"+.base.channel)")

	if [[ $expected_base == "$actual_base" ]]; then
		echo "Machine $machine has base $actual_base"
	else
		echo "Machine $machine has base $actual_base, expected $expected_base"
		exit 1
	fi
}

# Reboot the given machine and wait for it to restart.
reboot_machine() {
	local machine
	machine=$1

	juju ssh "$machine" 'sudo shutdown -r now' || true
	wait_for_machine_agent_status "$machine" "down"
	wait_for_machine_agent_status "$machine" "started"
}

test_upgrade_series_relation() {
	if [ -n "$(skip 'test_upgrade_series_relation')" ]; then
		echo "==> SKIP: Asked to skip upgrade series relation tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_upgrade_base_relation"
	)
}
