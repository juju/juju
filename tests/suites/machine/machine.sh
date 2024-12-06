test_log_permissions() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists
	file="${TEST_DIR}/test_log_permissions.log"
	ensure "correct-log" "${file}"

	juju deploy juju-qa-test source --base ubuntu@20.04

	wait_for "started" '.machines."0"."juju-status".current'

	check_contains "$(juju ssh 0 -- stat -c '%G' /var/log/juju/unit-source-0.log)" adm
	check_contains "$(juju ssh 0 -- stat -c '%a' /var/log/juju/unit-source-0.log)" 640

	check_contains "$(juju ssh 0 -- stat -c '%a' /var/log/juju/machine-0.log)" 640
	check_contains "$(juju ssh 0 -- stat -c '%G' /var/log/juju/machine-0.log)" adm

	# Clean up!
	destroy_model "correct-log"
}

test_logs() {
	if [ -n "$(skip 'test_logs')" ]; then
		echo "==> SKIP: Asked to skip test_logs tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "test_log_permissions"
	)
}
