test_unit() {
	if [ "$(skip 'test_unit')" ]; then
		echo "==> TEST SKIPPED: unit tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-units.log"

	bootstrap "test-units" "${file}"

	# Tests that need to be run are added here.
	test_unit_series

	destroy_controller "test-units"
}
