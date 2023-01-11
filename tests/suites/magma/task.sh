test_magma() {
	if [ "$(skip 'test_magma')" ]; then
		echo "==> TEST SKIPPED: Magma tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-magma.log"

	bootstrap "test-magma" "${file}"

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		microk8s disable metallb
		microk8s enable metallb:10.1.1.1-10.1.1.17
		test_deploy_magma
		;;
	*)
		echo "==> TEST SKIPPED: test_deploy_magma test runs on k8s only"
		;;
	esac

	# Magma takes too long to tear down (1h+), so forcibly destroy it
	export KILL_CONTROLLER=true
	destroy_controller "test-magma"
}
