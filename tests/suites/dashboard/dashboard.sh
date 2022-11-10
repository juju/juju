run_dashboard_deploy() {
	echo

	juju switch controller
	juju deploy juju-dashboard dashboard
	juju expose dashboard
	juju relate dashboard controller

	juju wait-for application dashboard
	sleep 5 # short wait for relation data to update
	open_dashboard

	# Switch to different model and test
	model_name="test-dashboard"
	juju add-model "${model_name}"
	open_dashboard

	destroy_model "${model_name}"
	juju switch controller
	juju remove-application dashboard
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
	juju dashboard &
	PID=$!
	sleep 2
	# TODO: capture url from dashboard output
	curl -L http://localhost:31666 | grep "Juju Dashboard"
	kill -SIGINT "$PID"
	# TODO: why isn't this killing the child ssh process?
	#   lsof -n -i | grep 31666
}
