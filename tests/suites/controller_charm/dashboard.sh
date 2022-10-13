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

	# Ensure controller charm still "active", not "error"
	local controller_charm_status
	controller_charm_status=$(juju status -m controller --format json | jq -r '.applications.controller."application-status".current')
	if [[ $controller_charm_status != 'active' ]]; then
		exit 1
	fi

	# juju dashboard
	# TODO: check that it prints the url etc
	# TODO: this command will block, how do we interrupt it?

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
