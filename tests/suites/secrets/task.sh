test_secrets() {
	if [ "$(skip 'test_secrets')" ]; then
		echo "==> TEST SKIPPED: secrets tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-secrets.log"

	if [[ -n ${OPERATOR_IMAGE_ACCOUNT:-} ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --config caas-image-repo=${OPERATOR_IMAGE_ACCOUNT}"
	fi
	# TODO: remove we do not need anymore.
	export JUJU_DEV_FEATURE_FLAGS=developer-mode

	bootstrap "test-secrets" "${file}"

	test_secrets_juju
	test_secrets_k8s
	test_secrets_vault

	destroy_controller "test-secrets"
}
