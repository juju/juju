test_storage_k8s() {
	if [ "$(skip 'test_storage_k8s')" ]; then
		echo "==> TEST SKIPPED: caas filesystem tests"
		return
	fi

	set_verbosity

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		echo "==> Checking for dependencies"
		check_dependencies juju

		kubectl config view --raw --flatten >"${TEST_DIR}"/kube.conf
		export KUBE_CONFIG="${TEST_DIR}"/kube.conf

		test_import_filesystem
		test_force_import_filesystem
		test_deploy_attach_storage
		test_add_unit_attach_storage
		test_add_unit_duplicate_pvc_exists
		test_add_unit_attach_storage_scaling_race_condition

		# Tests involving storage resize.
		test_scale_and_update_storage
		test_scale_and_update_storage_successive
		test_scale_app_with_updated_storage_self_healing
		test_scale_after_storage_update_crash
		test_scale_resumes_after_storage_update_missing_sts
		test_storage_update_after_scale_crash
		test_remove_app_while_storage_update_stuck
		;;
	*)
		echo "==> TEST SKIPPED: storage k8s tests, not a k8s provider"
		;;
	esac
}
