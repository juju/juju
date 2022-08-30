create_storage_pools() {
	echo

  model_name="test-storage"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	echo "Assessing create-pool"
	juju create-storage-pool -m "${model_name}" loopy loop size=1G
	juju create-storage-pool -m "${model_name}" rooty rootfs size=1G
	juju create-storage-pool -m "${model_name}" tempy tmpfs size=1G
	juju create-storage-pool -m "${model_name}" ebsy ebs size=1G
	# Assess the above created storage pools.
	echo "Assessing storage pool"
	if [ "${BOOTSTRAP_PROVIDER:-}" == "ec2" ]; then
		juju list-storage-pools -m "${model_name}" --format json | jq '.ebs.provider' | check "ebs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.["ebs-ssd"]["provider"]' | check "ebs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.tmpfs.provider' | check "tmpfs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.loop.provider' | check "loop"
		juju list-storage-pools -m "${model_name}" --format json | jq '.rootfs.provider' | check "rootfs"
	else
		juju list-storage-pools -m "${model_name}" --format json | jq '.rooty.provider' | check "rootfs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.tempy.provider' | check "tmpfs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.loop.provider' | check "loop"
		juju list-storage-pools -m "${model_name}" --format json | jq '.ebsy.provider' | check "ebs"
	fi

  destroy_model "${model_name}"
}


test_create_storage_pool() {
	if [ "$(skip 'test_create_storage_pool')" ]; then
		echo "==> TEST SKIPPED: create storage pools tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_create_storage_pools"
	)
}