test_remove_machine() {
	if [ "$(skip 'test_remove_machine')" ]; then
		echo "==> TEST SKIPPED: remove-machine tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju
	case "${BOOTSTRAP_PROVIDER:-}" in
	"aws" | "ec2")
		setup_awscli_credential
		check_dependencies aws
		;;
	"google" | "gce")
		setup_gcloudcli_credential
		check_dependencies gcloud
		;;
	"azure")
		check_dependencies az
		;;
	"lxd")
		if cloud_instance_removal_supported; then
			check_dependencies lxc
		fi
		;;
	esac

	file="${TEST_DIR}/test-remove-machine.log"
	bootstrap "test-remove-machine" "${file}"

	test_remove_non_controller_machines
	test_remove_controller_machine

	destroy_controller "test-remove-machine"
}
