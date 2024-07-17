run_machine_ssh() {
	# Echo out to ensure nice output to the test suite.
	echo

	# Add a new machine for the test to assert sshing into.
	juju add-machine -n 1
	wait_for_machine_agent_status "0" "started"

	# Generate a new ssh key pair for this test.
	ssh_key_file="${TEST_DIR}/machine-ssh-key"
	ssh_key_file_pub="${ssh_key_file}.pub"
	ssh-keygen -t ed25519 -f "$ssh_key_file" -C "isgreat@juju.is" -P ""

	# Add the SSH key and see that it comes back out in the list of user keys.
	juju add-ssh-key "$(cat $ssh_key_file_pub)"

	# Watch the debug log for the agent to assert that a key has been added to
	# the machine.
	timeout 5m juju debug-log --tail | grep -m 1 'adding ssh keys to authorized keys' || true

	# Check that the test can ssh to the machine with the new keypair and run a
	# command.
	check_contains "$(juju ssh 0 -i \"${ssh_key_file}\" echo foobar)" "foobar"
}

# test_machine_ssh is responsible for testing that adding authorized keys to a
# model traverse through the controller and down to the machine agents. After
# this has happened ssh access to a machine should be granted for the owner of
# the newly added public key.
test_machine_ssh() {
	if [ "$(skip 'test_machine_ssh')" ]; then
		echo "==> TEST SKIPPED: authorized keys machine ssh"
		return
	fi

	(
		set_verbosity

		run "run_machine_ssh"
	)
}
