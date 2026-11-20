test_cloud_gce() {
	if [ "$(skip 'test_cloud_gce')" ]; then
		echo "==> TEST SKIPPED: google gce"
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

	if [ -z "$(skip 'test_pro_images' 'test_deploy_gpu_instance' 'test_create_storage_pool' 'test_root_disk_source' 'test_instance_type_arch')" ]; then
		# An explicitly pinned BOOTSTRAP_REGION is honoured as-is;
		# otherwise ("auto" or unset) the region is probed: the suite runs
		# in the first candidate region offering the required machine types
		# and quota, and GCE infrastructure failures (out of stock, quota
		# exceeded) exclude the region and retry the suite in the next one.
		#
		# The suite shares one bootstrap for all subtests, so the declared
		# requirements are the union of what they need at their concurrent
		# peak: the controller (2 cores), the root_disk_source model
		# accumulating four coexisting deploys (two c3-standard-4 plus two
		# default 2-core machines: ~12 vCPU and 4 addresses) and the GPU
		# subtest's g2-standard-4 (4 vCPU, 1 L4 GPU), one address each.
		# G2_CPUS and C3_CPUS guard the machine families the subtests
		# consume; the controller's own family (picked by the provider
		# from PUBLIC_CLOUD_BOOTSTRAP_CONSTRAINTS) cannot be pre-checked
		# and is left to the retry loop. Candidates are ordered US first
		# (closest to the CI runners), EU as fallback; only G2-capable
		# regions are listed.
		GCE_REGION_CANDIDATES="${GCE_REGION_CANDIDATES:-us-east1 us-east4 us-central1 us-west1 us-west4 europe-west4 europe-west1}"
		GCE_REQUIRED_MACHINE_TYPES="${GCE_REQUIRED_MACHINE_TYPES:-g2-standard-4 c3-standard-4}"
		GCE_REQUIRED_QUOTAS="${GCE_REQUIRED_QUOTAS:-CPUS>=14 IN_USE_ADDRESSES>=6 NVIDIA_L4_GPUS>=1 G2_CPUS>=4 C3_CPUS>=8}"
		export GCE_REGION_CANDIDATES GCE_REQUIRED_MACHINE_TYPES GCE_REQUIRED_QUOTAS

		if [[ -n ${BOOTSTRAP_REGION:-} && ${BOOTSTRAP_REGION} != "auto" ]]; then
			bootstrap "test-cloud-gce" "${file}"
			cloud_gce_subtests
			destroy_controller "test-cloud-gce"
		else
			gce_run_with_region_fallback "test-cloud-gce" "${file}" cloud_gce_subtests
		fi
	fi

	# This test bootstraps a custom controller.
	test_serviceaccount_credential
}

cloud_gce_subtests() {
	test_pro_images
	test_deploy_gpu_instance
	test_create_storage_pool
	test_root_disk_source
	test_instance_type_arch
}
