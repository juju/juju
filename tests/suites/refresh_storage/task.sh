test_refresh_storage() {
	if [ "$(skip 'test_refresh_storage')" ]; then
		echo "==> TEST SKIPPED: Refresh storage tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-refresh-storage-ctl.log"

	bootstrap "test-refresh-storage-ctl" "${file}"

	test_refresh_charm_storage_user_override

	destroy_controller "test-refresh-storage-ctl"
}
