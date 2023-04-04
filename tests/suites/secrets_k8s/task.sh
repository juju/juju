test_secrets_k8s() {
	if [ "$(skip 'test_secrets_k8s')" ]; then
		echo "==> TEST SKIPPED: test_secrets_k8s tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-secrets-k8s.log"

	# TODO: remove feature flag when secret is fully ready.
	export JUJU_DEV_FEATURE_FLAGS=developer-mode

	bootstrap "test-secrets-k8s" "${file}"

	test_secrets

	destroy_controller "test-secrets-k8s"
}
