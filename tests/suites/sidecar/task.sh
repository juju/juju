test_sidecar() {
	if [ "$(skip 'test_sidecar')" ]; then
		echo "==> TEST SKIPPED: sidecar charm tests"
		return
	fi

	set_verbosity

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		test_deploy_and_remove_application
		test_deploy_and_force_remove_application
		test_pebble_notices
		;;
	*)
		echo "==> TEST SKIPPED: sidecar charm tests, not a k8s provider"
		;;
	esac
}
