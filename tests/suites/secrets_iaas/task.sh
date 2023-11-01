test_secrets_iaas() {
	if [ "$(skip 'test_secrets_iaas')" ]; then
		echo "==> TEST SKIPPED: test_secrets_iaas tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-secrets-iaas.log"

	bootstrap "test-secrets-iaas" "${file}"

	test_secrets_juju
	test_secrets_cmr
	test_secrets_vault
	test_secret_drain
	test_user_secret_drain

	# Takes too long to tear down, so forcibly destroy it
	export KILL_CONTROLLER=true
	destroy_controller "test-secrets-iaas"
}
