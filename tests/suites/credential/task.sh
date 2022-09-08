test_credential() {
	if [ "$(skip 'test_credential')" ]; then
		echo "==> TEST SKIPPED: credential tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-credentials.log"

	test_add_remove_credential
}
