test_kubeflow() {
	# The kubeflow bundle includes charms that declare "assumes: juju < 4.0.0"
	# and cannot be deployed on Juju 4.x. Fixing this requires a kubeflow bundle
	# update which is out of scope for Juju's own CI. Skipping until the bundle
	# is updated to support Juju 4.x (see JUJU-9794).
	echo "==> TEST SKIPPED: kubeflow tests (kubeflow bundle incompatible with Juju 4.x, see JUJU-9794)"
	return

	if [ "$(skip 'test_kubeflow')" ]; then
		echo "==> TEST SKIPPED: kubeflow tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-deploy-kubeflow.log"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		bootstrap "test-kubeflow" "${file}"

		if [[ ${BOOTSTRAP_CLOUD} == "microk8s" ]]; then
			sudo microk8s disable metallb || true # for local testing, this can be done out of band
			sudo microk8s enable "metallb:10.64.140.43-10.64.140.49" || true
		fi

		test_deploy_kubeflow
		;;
	*)
		echo "==> TEST SKIPPED: test_deploy_kubeflow test runs on k8s only"
		;;
	esac

	export KILL_CONTROLLER=true
	destroy_controller "test-kubeflow"
}
