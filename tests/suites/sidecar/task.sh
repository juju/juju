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
		test_pebble_checks
		test_credential_get_k8s
		test_rootless
		;;
	*)
		echo "==> TEST SKIPPED: sidecar charm tests, not a k8s provider"
		;;
	esac
}
