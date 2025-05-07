# This file contains functions to facilitate SSH connections through a jump host.
# The commands are useful for testing Juju's SSH proxy using both ssh and scp.

ssh_flags=(
	-o StrictHostKeyChecking=no
	-o UserKnownHostsFile=/dev/null
)

# Note the use of ProxyCommand below instead of the simpler -J flag.
# This is needed because -J issues an ssh process which doesn't
# inherit the arguements from the parent process. We want both ssh
# connections to ignore the host key check to ensure the test runs non-interactively.

make_proxy_command() {
	local jump_host=$1
	local ssh_port=17022

	echo "ssh ${ssh_flags[*]} -p $ssh_port $jump_host -W %h:%p"
}

ssh_wrapper_with_proxy() {
	local jump_host=$1
	shift # Remove jump_host from arguments

	ssh "${ssh_flags[@]}" -o ProxyCommand="$(make_proxy_command "$jump_host")" "$@"
}

scp_wrapper_with_proxy() {
	local jump_host=$1
	shift # Remove jump_host from arguments

	scp "${ssh_flags[@]}" -o ProxyCommand="$(make_proxy_command "$jump_host")" "$@"
}
