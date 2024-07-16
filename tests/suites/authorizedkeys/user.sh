
run_user_ssh_keys() {
	# Echo out to ensure nice output to the test suite.
    echo

    ssh_key_file="${TEST_DIR}/juju-ssh-key"
    ssh_key_file_pub="${ssh_key_file}.pub"

    ssh-keygen -t ed25519 -f "$ssh_key_file" -C "isgreat@juju.is" -P ""
    fingerprint=$(ssh-keygen -E md5 -lf "${ssh_key_file_pub}" | cut -f 2 -d ' ' | cut -f 2- -d ':')

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
}

test_user_ssh_keys() {
	if [ "$(skip 'test_user_ssh_keys')" ]; then
		echo "==> TEST SKIPPED: authorisedkeys user ssh keys"
		return
	fi

	(
		set_verbosity

		run "run_user_ssh_keys"
	)
}