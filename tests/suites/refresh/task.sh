test_refresh() {
	if [ "$(skip 'test_refresh')" ]; then
		echo "==> TEST SKIPPED: Refresh tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-refresh-ctl.log"

	bootstrap "test-refresh-ctl" "${file}"

	test_basic
	test_switch

	destroy_controller "test-refresh-ctl"
}
