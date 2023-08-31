test_controllercharm() {
	if [ "$(skip 'test_controllercharm')" ]; then
		echo "==> TEST SKIPPED: controller charm tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	test_prometheus
}