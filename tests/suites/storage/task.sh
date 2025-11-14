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
	test_model_storage_block
	test_model_storage_filesystem
	test_persistent_storage
	test_application_storage

	destroy_controller "test-storage"
}
