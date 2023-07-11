test_smoke_k8s() {
	if [ "$(skip 'test_smoke_k8s')" ]; then
		echo "==> TEST SKIPPED: k8s smoke tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-smoke-k8s.log"
	echo "====> Logging to ${file}"
	bootstrap "test-smoke-k8s" "${file}"

	test_deploy "${file}"

	destroy_controller "test-smoke-k8s"
}
