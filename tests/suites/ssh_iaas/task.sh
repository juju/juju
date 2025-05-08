test_ssh_iaas() {
	if [ "$(skip 'test_ssh')" ]; then
		echo "==> TEST SKIPPED: ssh tests"
		return
	fi

	set_verbosity

	if [ "${BOOTSTRAP_PROVIDER:-}" == "k8s" ]; then
		echo "==> TEST SKIPPED: iaas ssh tests, not valid using a k8s provider"
		return
	fi

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-ssh-iaas.log"

	JUJU_DEV_FEATURE_FLAGS=ssh-jump bootstrap "test-ssh-iaas" "${file}"

	# Add tests here
	test_ssh

	destroy_controller "test-ssh-iaas"
}
