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
