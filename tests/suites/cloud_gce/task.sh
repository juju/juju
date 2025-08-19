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

	echo "==> Checking for dependencies"

	check_dependencies gcloud

	file="${TEST_DIR}/test-cloud-gce.log"

	bootstrap "test-cloud-gce" "${file}"

	test_pro_images

	destroy_controller "test-cloud-gce"
}
