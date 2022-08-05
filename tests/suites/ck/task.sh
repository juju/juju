test_ck() {
	if [ "$(skip 'test_ck')" ]; then
		echo "==> TEST SKIPPED: CK tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-ck.log"

	if [[ -n ${OPERATOR_IMAGE_ACCOUNT:-} ]]; then
		export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --config caas-image-repo=${OPERATOR_IMAGE_ACCOUNT}"
	fi
	bootstrap "test-ck" "${file}"

	test_deploy_ck

	if [[ ${BOOTSTRAP_REUSE:-} != "true" ]]; then
		# CK takes too long to tear down (1h+), so forcibly destroy it
		juju kill-controller -y -t 0 "${BOOTSTRAPPED_JUJU_CTRL_NAME}" || true
	fi
}
