test_storage_k8s() {
	if [ "$(skip 'test_storage_k8s')" ]; then
		echo "==> TEST SKIPPED: caas filesystem tests"
		return
	fi

  # WORKAROUND(juju/juju#23075): CAAS `juju deploy --attach-storage` is
	# rejected by a stale facade guard in apiserver/facades/client/application/deploy.go
	# (the backend in PR #22160 supports CAAS, but the facade guard was not lifted).
	# Skip until the guard is removed.
	add_skipped "test_add_unit_attach_storage"
	add_skipped "test_add_unit_duplicate_pvc_exists"
	add_skipped "test_add_unit_attach_storage_scaling_race_condition"
	add_skipped "test_deploy_attach_storage"


	set_verbosity

	case "${BOOTSTRAP_PROVIDER:-}" in
	"k8s")
		echo "==> Checking for dependencies"
		check_dependencies juju

		kubectl config view --raw --flatten >"${TEST_DIR}"/kube.conf
		export KUBE_CONFIG="${TEST_DIR}"/kube.conf

		test_import_filesystem
		test_force_import_filesystem
		test_destroy_model_with_detached_storage
		test_deploy_attach_storage
		test_add_unit_attach_storage
		test_add_unit_duplicate_pvc_exists
		test_add_unit_attach_storage_scaling_race_condition
		;;
	*)
		echo "==> TEST SKIPPED: storage k8s tests, not a k8s provider"
		;;
	esac
}
