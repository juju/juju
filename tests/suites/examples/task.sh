test_examples() {
	if [ "$(skip 'test_examples')" ]; then
		echo "==> TEST SKIPPED: example tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-example.log"

	bootstrap "test-example" "${file}"

	# Test that need to be run are added here!
	test_example
	test_other

	destroy_controller "test-example"
}
