test_cmr() {
	if [ "$(skip 'test_cmr')" ]; then
		echo "==> TEST SKIPPED: cross-model relations tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju go

	# Only bootstrap the shared controller when at least one test that uses it
	# will run. This avoids an unnecessary bootstrap/destroy cycle when only
	# test_offer_find_external_user is selected (it manages its own controller).
	if [ -z "$(skip 'test_offer_consume')" ] || [ -z "$(skip 'test_offer_find_non_admin')" ]; then
		file="${TEST_DIR}/test-cmr.log"

		bootstrap "test-cmr" "${file}"

		test_offer_consume
		test_offer_find_non_admin

		destroy_controller "test-cmr"
	fi

	# test_offer_find_external_user bootstraps its own dedicated controller because it
	# requires identity-url/identity-public-key config and the go toolchain.
	test_offer_find_external_user
}
