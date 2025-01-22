test_constraints_common() {
	if [ "$(skip 'test_constraints_common')" ]; then
		echo "==> TEST SKIPPED: constraints common"
		return
	fi

	(
		set_verbosity

		file="${TEST_DIR}/test-constraints.txt"
		ensure "test-constraints" "${file}"

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			run "run_constraints_lxd"
			;;
		"openstack")
			run "run_constraints_openstack"
			;;
		"ec2")
			run "run_constraints_aws"
			;;
		"microk8s")
			echo "==> TEST SKIPPED: constraints - there are no test for k8s cloud"
			;;
		*)
			run "run_constraints_vm"
			;;
		esac

		destroy_controller "test-constraints"
	)
}

# test_constraints_model is concerned with
test_constraints_model() {
	if [ "$(skip 'test_constraints_model')" ]; then
		echo "==> TEST SKIPPED: constraints model"
		return
	fi

	(
		set_verbosity

		run "run_constraints_model_bootstrap"
	)
}