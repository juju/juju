test_controllercharm() {
	if [ "$(skip 'test_controllercharm')" ]; then
		echo "==> TEST SKIPPED: controller charm tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	# Since we are testing the controller charm here, we want to do a fresh
	# bootstrap for every subtest.

	test_prometheus
}
