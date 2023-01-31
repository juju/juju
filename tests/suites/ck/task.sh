test_ck() {
	if [ "$(skip 'test_ck')" ]; then
		echo "==> TEST SKIPPED: CK tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-ck.log"

	bootstrap "test-ck" "${file}"

	test_deploy_ck

	# CK takes too long to tear down (1h+), so forcibly destroy it
	export KILL_CONTROLLER=true
	destroy_controller "test-ck"
}
