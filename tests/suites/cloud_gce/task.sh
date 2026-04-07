test_cloud_gce() {
	if [ "$(skip 'test_cloud_gce')" ]; then
		echo "==> TEST SKIPPED: azure gce"
		return
	fi

	set_verbosity

	if [ "${BOOTSTRAP_PROVIDER}" != "gce" ]; then
		echo "==> TEST SKIPPED: gce cloud tests, not using gce"
		return
	fi

	setup_gcloudcli_credential

	echo "==> Checking for dependencies"
	check_dependencies juju gcloud

	file="${TEST_DIR}/test-cloud-gce.log"

	if [ -z "$(skip 'test_pro_images' 'test_deploy_gpu_instance' 'test_create_storage_pool' 'test_root_disk_source')" ]; then
		bootstrap "test-cloud-gce" "${file}"

		test_pro_images
		test_deploy_gpu_instance
		test_create_storage_pool
		test_root_disk_source

		destroy_controller "test-cloud-gce"
	fi

	# This test bootstraps a custom controller.
	test_serviceaccount_credential
}
