run_dashboard_deploy() {
	echo

	juju switch controller
	juju deploy juju-dashboard dashboard
	juju expose dashboard
	juju relate dashboard controller

	wait_for "controller" "$(active_condition "dashboard")"

	# verify juju dashboard fails as expected
	#open_dashboard
	output=$(juju dashboard 2>&1 || true)
	check_contains "$output" 'not implemented'

	# Switch to different model and test
	model_name="test-dashboard"
	juju add-model "${model_name}"

	# verify juju dashboard fails as expected
	#open_dashboard
	output=$(juju dashboard 2>&1 || true)
	check_contains "$output" 'not implemented'

	destroy_model "${model_name}"
	juju switch controller
	juju remove-application --no-prompt dashboard
}

test_dashboard_deploy() {
	if [ "$(skip 'test_dashboard_deploy')" ]; then
		echo "==> TEST SKIPPED: deploy Juju Dashboard"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_dashboard_deploy"
	)
}

open_dashboard() {
	# The DashboardConnectionInfo call in `juju dashboard` is
	# currently returning an NotImplemented error as the functionality
	# needs to be reimplemented in the controller charm.
	# TODO update test once a solution is available.
	#
	juju dashboard &
	PID=$!
	sleep 10
	# TODO: capture url from dashboard output
	curl -L http://localhost:31666 | grep "Juju Dashboard"
	kill -SIGINT "$PID"
	# TODO: why isn't this killing the child ssh process?
	#   lsof -n -i | grep 31666
}
