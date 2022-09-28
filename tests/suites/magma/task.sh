test_magma() {
	if [ "$(skip 'test_magma')" ]; then
		echo "==> TEST SKIPPED: Magma tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-magma.log"

	bootstrap "test-magma" "${file}"

	test_deploy_magma

	destroy_controller "test-magma"
}
