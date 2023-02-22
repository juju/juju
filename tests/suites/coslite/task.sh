test_coslite() {
	if [ "$(skip 'test_coslite')" ]; then
		echo "==> TEST SKIPPED: COS Lite tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test_coslite.log"

	bootstrap "test-coslite" "${file}"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		# disable metallb then enable it with a new set of out ipaddr
		microk8s disable metallb
		IPADDR=$(ip -4 -j route get 2.2.2.2 | jq -r '.[] | .prefsrc')
		microk8s enable metallb:"$IPADDR"-"$IPADDR"
		microk8s kubectl rollout status deployments/hostpath-provisioner -n kube-system -w
		microk8s kubectl rollout status deployments/coredns -n kube-system -w
		microk8s kubectl rollout status daemonset.apps/speaker -n metallb-system -w

		test_deploy_coslite
		;;
	*)
		echo "==> TEST SKIPPED: test_deploy_coslite test runs on k8s only"
		;;

	esac

	# TODO(basebandit): remove KILL_CONTROLLER once model teardown has been fixed for k8s models.
	export KILL_CONTROLLER=true
	destroy_controller "test-coslite"
}
