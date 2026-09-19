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
	if [ -z "$(skip 'test_offer_consume' 'test_offer_find_non_admin' 'test_offer_consume_migrate')" ]; then
		file="${TEST_DIR}/test-cmr.log"

		bootstrap "test-cmr" "${file}"

		test_offer_consume
		test_offer_find_non_admin
		test_offer_consume_migrate

		if juju controllers --format=json 2>/dev/null |
			yq -r 'select(.controllers) | .controllers | keys | .[]' |
			grep "test-cmr" || juju models --format=json 2>/dev/null |
			yq -r '.models | .[] | .["short-name"]' |
			grep -x "test-cmr"; then
			destroy_controller "test-cmr"
		fi
	fi

	if [ -z "$(skip 'test_cmr_integrity')" ]; then
		# test_cmr_integrity bootstraps its own dedicated controller.
		test_cmr_integrity
	fi

	if [ -z "$(skip 'test_offer_find_external_user')" ]; then
		# test_offer_find_external_user bootstraps its own dedicated controller because it
		# requires identity-url/identity-public-key config and the go toolchain.
		test_offer_find_external_user
	fi
}
