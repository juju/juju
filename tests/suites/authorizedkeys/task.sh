test_authorizedkeys() {
	if [ "$(skip 'test_authorizedkeys')" ]; then
		echo "==> TEST SKIPPED: authorizedkeys"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	log_file="${TEST_DIR}/authorisedkeys.log"

	ensure "authorisedkeys" "$log_file"

	test_user_ssh_keys
	test_machine_ssh

	destroy_controller "authorisedkeys"
}