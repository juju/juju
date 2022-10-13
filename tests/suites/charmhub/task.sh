test_charmhub() {
	if [ "$(skip 'test_charmhub')" ]; then
		echo "==> TEST SKIPPED: Charm Hub tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-charmhub.log"

	# These tests don't need a controller.
	test_charmhub_find
	test_charmhub_info

	bootstrap "test-charmhub" "${file}"

	test_charmhub_download

	destroy_controller "test-charmhub"
}
