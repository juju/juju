test_constraints_common() {
	if [ "$(skip 'test_constraints_common')" ]; then
		echo "==> TEST SKIPPED: constraints"
		return
	fi

	(
		set_verbosity

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
	)
}
