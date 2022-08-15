test_constraints() {
	if [ "$(skip 'test_constraints')" ]; then
		echo "==> TEST SKIPPED: constraints tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-constraints.txt"

	bootstrap "test-constraints" "${file}"

	test_constraints_common

	destroy_controller "test-constraints"
}
