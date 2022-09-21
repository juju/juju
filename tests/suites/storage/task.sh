test_storage() {
	if [ "$(skip 'test_storage')" ]; then
		echo "==> TEST SKIPPED: storage tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-storage.log"

	bootstrap "test-storage" "${file}"

	test_charm_storage
	test_persistent_storage

	destroy_controller "test-storage"
}
