run_ssh() {
	echo

	model_name='model-ssh'
	juju --show-log add-model "$model_name"
	model_uuid=$(juju show-model $model_name --format json | jq -r ".[\"${model_name}\"][\"model-uuid\"]")

	app_name="ubuntu"
	juju deploy $app_name
	wait_for $app_name "$(active_idle_condition $app_name 0 0)"

	controller_name=$(juju controllers --format json | jq -r '."current-controller"')
	controller_address=$(juju show-controller --format json | jq -r ".[\"${controller_name}\"][\"details\"][\"api-endpoints\"][0]" | cut -d ':' -f 1)

	machine_hostname=0.$model_uuid.juju.local
	unit_hostname=0.$app_name.$model_uuid.juju.local

	check_ssh_using_openssh "$controller_address" "$machine_hostname"
	check_ssh_using_openssh "$controller_address" "$unit_hostname"

	check_scp_using_openssh "$controller_address" "$machine_hostname"
	check_scp_using_openssh "$controller_address" "$unit_hostname"

	# TODO: Add tests for the Juju CLI below

	destroy_model "${model_name}"
}

check_ssh_using_openssh() {
	controller_address=${1}
	virtual_hostname=${2}
	test_file="test-ssh.txt"

	# Check that we can write a file on the remote host then read it back.
	jump_host=admin@"$controller_address"
	ssh_no_hostkey_check "$jump_host" ubuntu@"$virtual_hostname" "echo hello > $test_file"
	output=$(ssh_no_hostkey_check "$jump_host" ubuntu@"$virtual_hostname" "cat $test_file")
	check_contains "$output" hello
}

check_scp_using_openssh() {
	controller_address=${1}
	virtual_hostname=${2}
	scp_file="test-scp.txt"

	# Check that we can copy a file to the remote host then copy it back.
	echo hello > $scp_file
	jump_host=admin@"$controller_address"
	scp_no_hostkey_check "$jump_host" test-scp.txt ubuntu@"$virtual_hostname":$scp_file
	scp_no_hostkey_check "$jump_host" ubuntu@"$virtual_hostname":$scp_file $scp_file
	check_contains "$(cat $scp_file)" hello
}

ssh_no_hostkey_check() {
	jump_host=${1}
	ssh_port=17022

	# Note the use of ProxyCommand instead of the simpler -J flag.
	# Needed because -J issues another ssh process which doesn't
	# inherit the arguements from the parent process. We need both ssh
	# processes to ignore the host key check to ensure the process is not interactive.
	proxy_command="ssh ${ssh_flags[*]} -p $ssh_port $jump_host -W %h:%p"
	ssh "${ssh_flags[@]}" -o ProxyCommand="$proxy_command" "${@:2}"
}

scp_no_hostkey_check() {
	jump_host=${1}
	ssh_port=17022

	proxy_command="ssh ${ssh_flags[*]} -p $ssh_port $jump_host -W %h:%p"
	scp "${ssh_flags[@]}" -o ProxyCommand="$proxy_command" "${@:2}"
}

ssh_flags=(
	-o StrictHostKeyChecking=no
	-o UserKnownHostsFile=/dev/null
)

test_ssh() {
	if [ "$(skip 'test_ssh')" ]; then
		echo "==> TEST SKIPPED: test_ssh"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_ssh"
	)
}
