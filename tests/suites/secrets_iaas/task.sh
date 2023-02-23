test_secrets_iaas() {
	if [ "$(skip 'test_secrets_iaas')" ]; then
		echo "==> TEST SKIPPED: test_secrets_iaas tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-secrets-iaas.log"

	# TODO: remove feature flag when secret is fully ready.
	export JUJU_DEV_FEATURE_FLAGS=developer-mode

	bootstrap "test-secrets-iaas" "${file}"

	test_secrets_vault
	test_secrets_juju

	destroy_controller "test-secrets-iaas"
}
