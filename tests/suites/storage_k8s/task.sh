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

		microk8s config >"${TEST_DIR}"/kube.conf
		export KUBE_CONFIG="${TEST_DIR}"/kube.conf

		export JUJU_DEV_FEATURE_FLAGS=k8s-attach-storage
		test_import_filesystem
		test_force_import_filesystem
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
