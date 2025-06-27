test_smoke_k8s_psql() {
	if [ "$(skip 'test_smoke_k8s_psql')" ]; then
		echo "==> TEST SKIPPED: k8s postgresql smoke tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-smoke-k8s-psql.log"
	echo "====> Logging to ${file}"
	bootstrap "test-smoke-k8s-psql" "${file}"

	test_deploy_postgresql "${file}"

	destroy_controller "test-smoke-k8s-psql"
}
