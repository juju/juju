run_dashboard() {
	echo

	file="${TEST_DIR}/test-dashboard.log"

	ensure "test-dashboard" "${file}"

	juju switch controller
	juju offer controller:dashboard dashboard
	juju switch test-dashboard
	juju deploy juju-dashboard --channel=beta
	juju relate juju-dashboard controller.dashboard
	wait_for "juju-dashboard" "$(idle_condition "juju-dashboard")"
	# TODO: ensure controller charm still "active", not "error"
	# TODO: run the juju dashboard command, check that it prints the url etc

	destroy_model "test-dashboard"
}

test_dashboard() {
	if [ "$(skip 'test_dashboard')" ]; then
		echo "==> TEST SKIPPED: dashboard relation"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_dashboard"
	)
}
