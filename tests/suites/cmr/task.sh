test_cmr() {
	if [ "$(skip 'test_cmr')" ]; then
		echo "==> TEST SKIPPED: cross-model relations tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-cmr.log"

	bootstrap "test-cmr" "${file}"

	test_offer_consume

	destroy_controller "test-cmr"
}
