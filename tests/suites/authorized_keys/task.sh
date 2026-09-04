test_authorized_keys() {
	if [ "$(skip 'test_authorized_keys')" ]; then
		echo "==> TEST SKIPPED: authorized_keys"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	log_file="${TEST_DIR}/authorized_keys.log"

	ensure "authorizedkeys" "$log_file"

	test_user_ssh_keys
	test_machine_ssh
	test_bootstrap_authorized_keys

	destroy_controller "authorizedkeys"
}
