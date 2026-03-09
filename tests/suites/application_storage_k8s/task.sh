test_application_storage_k8s() {
	if [ "$(skip 'test_application_storage_k8s')" ]; then
		echo "==> TEST SKIPPED: caas application storage tests"
		return
	fi

	set_verbosity

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		echo "==> Checking for dependencies"
		check_dependencies juju

		test_scale_app_with_updated_storage
		test_scale_app_with_updated_storage_self_healing
		test_scale_after_storage_update_crash
		test_scale_resumes_after_storage_update_missing_sts
		;;
	*)
		echo "==> TEST SKIPPED: application storage k8s tests, not a k8s provider"
		;;
	esac
}
