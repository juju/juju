test_remove_machine() {
	if [ "$(skip 'test_remove_machine')" ]; then
		echo "==> TEST SKIPPED: remove-machine tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-remove-machine.log"
	bootstrap "test-remove-machine" "${file}"

	test_remove_non_controller_machines
	test_remove_controller_machine

	destroy_controller "test-remove-machine"
}
