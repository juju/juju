# Deploy the dummy-sink charm on an earlier series, then check that we can
# upgrade the series, and that the charm still behaves correctly.
run_upgrade_series_relation() {
	local start_series end_series
	start_series="focal"
	end_series="jammy"

	# Setup
	ensure "test-upgrade-series-relation" "${TEST_DIR}/test-upgrade-series-relation.log"
	juju deploy ./testcharms/charms/dummy-sink --series $start_series
	juju deploy ./testcharms/charms/dummy-source --series $end_series
	juju relate dummy-sink dummy-source
	juju config dummy-source token=Canonical

	# Check pre-conditions
	wait_for "Canonical" "$(workload_status 'dummy-sink' 0).message"
	assert_machine_series 0 $start_series

	# Upgrade the machine
	juju upgrade-series 0 prepare $end_series -y
	reboot_machine 0
	echo "Upgrading machine..."
	echo "See ${TEST_DIR}/do-release-upgrade.log for progress."
	# TODO: remove -d flag once Ubuntu 22.04.1 is released
	juju ssh 0 'sudo do-release-upgrade -d -f DistUpgradeViewNonInteractive' &>"${TEST_DIR}/do-release-upgrade.log" || true
	reboot_machine 0
	juju upgrade-series 0 complete

	# Check post-conditions
	wait_for "Canonical" "$(workload_status 'dummy-sink' 0).message"
	assert_machine_series 0 $end_series
	# Check post-series-upgrade hook has run
	juju show-status-log dummy-sink/0 | grep 'post-series-upgrade'
}

# Assert the given machine has the given series.
assert_machine_series() {
	local machine expected_series actual_series
	machine=$1
	expected_series=$2
	actual_series=$(juju status --format=json | jq -r ".machines[\"$machine\"].series")

	if [[ $expected_series == "$actual_series" ]]; then
		echo "Machine $machine has series $actual_series"
	else
		echo "Machine $machine has series $actual_series, expected $expected_series"
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

		run "run_upgrade_series_relation"
	)
}
