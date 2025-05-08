test_ssh_k8s() {
	if [ "$(skip 'test_ssh')" ]; then
		echo "==> TEST SKIPPED: ssh tests"
		return
	fi

	set_verbosity

	if [ "${BOOTSTRAP_PROVIDER:-}" != "k8s" ]; then
		echo "==> TEST SKIPPED: caas ssh tests, not a k8s provider"
		return
	fi

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-ssh-k8s.log"

	JUJU_DEV_FEATURE_FLAGS=ssh-jump bootstrap "test-ssh-k8s" "${file}"

	# Add tests here
	test_ssh

	destroy_controller "test-ssh-k8s"
}
