test_ssh() {
	if [ "$(skip 'test_ssh')" ]; then
		echo "==> TEST SKIPPED: SSH and SCP tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-ssh.log"
	bootstrap "test-ssh" "${file}"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		test_ssh_k8s
		;;
	*)
		test_ssh_machine
		;;
	esac

	destroy_controller "test-ssh"
}
