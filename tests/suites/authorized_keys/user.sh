run_user_ssh_keys() {
	# Echo out to ensure nice output to the test suite.
	echo

	ssh_key_file="${TEST_DIR}/juju-ssh-key"
	ssh_key_file_pub="${ssh_key_file}.pub"

	ssh-keygen -t ed25519 -f "$ssh_key_file" -C "isgreat@juju.is" -P ""
	fingerprint=$(ssh-keygen -lf "${ssh_key_file_pub}" | cut -f 2 -d ' ')

	# Add the SSH key and see that it comes back out in the list of user keys.
	juju add-ssh-key "$(cat $ssh_key_file_pub)"
	check_contains "$(juju ssh-keys)" "$fingerprint"

	# Remove the SSH key by fingerprint
	juju remove-ssh-key "${fingerprint}"
	check_not_contains "$(juju ssh-keys)" "${fingerprint}"

	# Add the SSH key and see that it comes back out in the list of user keys.
	juju add-ssh-key "$(cat $ssh_key_file_pub)"
	check_contains "$(juju ssh-keys)" "$fingerprint"

	# Remove the SSH key by comment
	juju remove-ssh-key isgreat@juju.is
	check_not_contains "$(juju ssh-keys)" "${fingerprint}"

	# Import the ssh keys for tlm from Github.
	juju import-ssh-key gh:tlm
	check_contains "$(juju ssh-keys --full)" "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHxsBSstfw6+55P/YPS8PyH6m58hxt3q2RK2OP1P6J/2"

	# Import the ssh keys for wallyworld from Launchpad
	juju import-ssh-key lp:wallyworld
	check_contains "$(juju ssh-keys --full)" "ssh-rsa AAAAB3NzaC1yc2EAAAABIwAAAQEA4OrXnYsxashjP64y5heB1jCgCERz4cTExsqY6n1ANFXP8AlIxLYHx/g4EE1of/DQ0+uDtimQjJfhvwoglmNkOW4WdWQtaFr1qhMivtSDXEnXI7RZQue9xqH6B3u8yweMqMjqr5mLqJ5eY1HEoFtLBh3tHPHKNM62w/Eb2LLCD2JblbuHmFvLnwxGWNp0jMU69DE/bDvKtmOx4idXBGnqTImOCcDTaNy1srSEiJIprwYqJSOXO61pIs9COQVG1EOadqqvgBE0koITMFPPIWm4dBxbh2DREVFSZIz6DuwPwWXaNk8YqcGH5bU4Y7o6I0iUyVrKT4yG0AjMNc1BaEocGQ=="
}

test_user_ssh_keys() {
	if [ "$(skip 'test_user_ssh_keys')" ]; then
		echo "==> TEST SKIPPED: authorized keys user ssh keys"
		return
	fi

	(
		set_verbosity

		run "run_user_ssh_keys"
	)
}
