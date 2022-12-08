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
		# Charmed kubeflow 1.6 only supports k8s 1.22
		# https://charmed-kubeflow.io/docs/install
		microk8s kubectl version -o json | jq -r '.serverVersion | .major+"."+.minor' | check "1.22"

		bootstrap "test-kubeflow" "${file}"

		microk8s disable metallb
		microk8s enable "metallb:10.64.140.43-10.64.140.49"

		KUBECONFIG="$(mktemp)"
		microk8s config >"${KUBECONFIG}"
		export KUBECONFIG

		test_deploy_kubeflow
		;;
	*)
		echo "==> TEST SKIPPED: test_deploy_kubeflow test runs on k8s only"
		;;
	esac

	export KILL_CONTROLLER=true
	destroy_controller "test-kubeflow"
}
