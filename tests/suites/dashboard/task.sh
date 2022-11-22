test_dashboard() {
	if [ "$(skip 'test_dashboard')" ]; then
		echo "==> TEST SKIPPED: Juju Dashboard tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju
	check_dependencies curl

	file="${TEST_DIR}/test-dashboard-ctl.log"

	# TODO: don't need "test-dashboard-ctl" to be created
	bootstrap "test-dashboard-ctl" "${file}"

	test_dashboard_deploy
	# TODO: test dashboard deploy in a different model/controller
	#   (i.e. cross-model / cross-controller relation)

	destroy_controller "test-dashboard-ctl"
}
