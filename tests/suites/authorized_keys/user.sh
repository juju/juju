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

	# Import the ssh keys for jujubot from Github.
	juju import-ssh-key gh:jujubot
	check_contains "$(juju ssh-keys --full)" "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAICZWEX5Y5o8UWJIutFgGlO4Y4LKmHcvRlgqYLxkBdlqn"

	# Import the ssh keys for juju-qa-bot from Launchpad
	juju import-ssh-key lp:juju-qa-bot
	check_contains "$(juju ssh-keys --full)" "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQCwxvks5knYCgy3FVzmrVG6MdBZOR5xlnewsWUtumJ3+E/nioms6jiRogzJJsfxXj/2mH0zr+zgjw0QEaVsditk7ambOIKt65HwhW1YGFX0NDw8XKBZBfD2EXOGbot5Bv2yWae3wydmY2f6SvtuTgdxcVbdMldnsGO50LSRMNDIyVdTZFrjDUyDXL7o66Nd1T5ioyZ5HwgqqLXWdzy4ZkI5UzeSowrZ9zMJlKcrPfDOmb7Xxgmod1xjATKj/BXQv5T5xhPiIRoHYGmuk2FlvWywVFjMyzJGr/AW4XyZ8/4c3541eQafJ1wcl9R8NdUfrpKDYbkgb6v2wINrKms0jz8GiVwqc9++KnCI6QvvBRmGgF3J9aeWUYItSg6h+9WUzD4/baxZl+KYENR8wOzZQ8NGukrTaqQYub4yaTcEX8EJ6hhvg/BotCvShozCBDijvbXPAeplCYt3yxUmy4m/TC7bKrAOZLbIV9roTST3XwT3OoQlbj+qyfqN0uZSaLLCrHgNv7fkRkWNMdJd5d2jOcEB+y9KKOmfQO0QH1kzrsRnTNOCmoaKeqLz5uBABVkCsAuoiqcS2qPwvzQu57pHxblmD8tVRGnrqJUztOdXZu7cGxG5ZWU8vk/dqv6SlV8xK2RH0LIVgtjZDzY4GmTo27UU55qo1jKBA2fuWIlyhZEwxw=="
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
