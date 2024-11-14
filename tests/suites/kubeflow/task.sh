test_kubeflow() {
	if [ "$(skip 'test_kubeflow')" ]; then
		echo "==> TEST SKIPPED: kubeflow tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju charmcraft

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
