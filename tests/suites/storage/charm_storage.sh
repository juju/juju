run_charm_storage() {
	echo

	model_name="test-charm-storage"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	echo "Assess create-storage-pool"
	juju create-storage-pool -m "${model_name}" loopy loop size=1G
	juju create-storage-pool -m "${model_name}" rooty rootfs size=1G
	juju create-storage-pool -m "${model_name}" tempy tmpfs size=1G
	juju create-storage-pool -m "${model_name}" ebsy ebs size=1G
	echo "create-storage-pool PASSED"

	# Assess the above created storage pools.
	echo "Assessing storage pool"
	if [ "${BOOTSTRAP_PROVIDER:-}" == "ec2" ]; then
		juju storage-pools -m "${model_name}" --format json | jq '.ebs | .provider' | check "ebs"
		juju storage-pools -m "${model_name}" --format json | jq '.["ebs-ssd"] | .provider' | check "ebs"
		juju storage-pools -m "${model_name}" --format json | jq '.tmpfs | .provider' | check "tmpfs"
		juju storage-pools -m "${model_name}" --format json | jq '.loop | .provider' | check "loop"
		juju storage-pools -m "${model_name}" --format json | jq '.rootfs | .provider' | check "rootfs"
	else
		juju storage-pools -m "${model_name}" --format json | jq '.rooty | .provider' | check "rootfs"
		juju storage-pools -m "${model_name}" --format json | jq '.tempy | .provider' | check "tmpfs"
		juju storage-pools -m "${model_name}" --format json | jq '.loopy | .provider' | check "loop"
		juju storage-pools -m "${model_name}" --format json | jq '.ebsy | .provider' | check "ebs"
	fi
	echo "Storage pool PASSED"

	# Assess charm storage with the filesystem storage provider
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-fs --series jammy --storage data=rootfs,1G
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-lp --series jammy --storage disks=loop,1G
	juju add-storage -m "${model_name}" dummy-storage-lp/0 disks=1
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-fs dummy-storage-tp --series jammy --storage data=tmpfs,1G
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-fs dummy-storage-np --series jammy --storage data=1G
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-fs dummy-storage-mp --series jammy --storage data=1G

	echo "Assessing filesystem rootfs"
	# assert the storage kind name
	assert_storage "filesystem" "$(kind_name "data" 0)"
	# assert the storage status
	assert_storage "alive" "$(life_status "data" 0)"
	# assert the storage label name
	assert_storage "data/0" "$(label 0)"
	# assert the unit attachment name
	assert_storage "dummy-storage-fs/0" "$(unit_attachment "data" 0 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "data" 0 "dummy-storage-fs" 0)"
	wait_for_storage "attached" "$(filesystem_status 0 0).current"
	# assert the filesystem size
	requested_storage=1024
	acquired_storage=$(juju storage --format json | jq '.filesystems | .["0/0"] | select(.pool=="rootfs") | .size ')
	if [ "$requested_storage" -gt "$acquired_storage" ]; then
		echo "acquired storage size $acquired_storage should be greater than the requested storage $requested_storage"
		exit 1
	fi
	echo "Filesystem rootfs PASSED"

	# Assess charm storage with the filesystem storage provider
	echo "Assessing block loop disk 1"
	# assert the storage kind name
	assert_storage "block" "$(kind_name "disks" 1)"
	# assert the storage status
	assert_storage "alive" "$(life_status "disks" 1)"
	# assert the storage label name
	assert_storage "disks/1" "$(label 4)"
	# assert the unit attachment name
	assert_storage "dummy-storage-lp/0" "$(unit_attachment "disks" 1 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "disks" 1 "dummy-storage-lp" 0)"
	echo "Block loop disk 1 PASSED"

	echo "Assessing add storage block loop disk 2"
	# assert the storage kind name
	assert_storage "block" "$(kind_name "disks" 2)"
	# assert the storage status
	assert_storage "alive" "$(life_status "disks" 2)"
	# assert the storage label name
	assert_storage "disks/2" "$(label 5)"
	# assert the unit attachment name
	assert_storage "dummy-storage-lp/0" "$(unit_attachment "disks" 2 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "disks" 2 "dummy-storage-lp" 0)"
	echo "Block loop disk 2 PASSED"

	echo "Assessing filesystem tmpfs"
	# assert the storage kind name
	assert_storage "filesystem" "$(kind_name "data" 3)"
	# assert the storage status
	assert_storage "alive" "$(life_status "data" 3)"
	# assert the storage label name
	assert_storage "data/3" "$(label 1)"
	# assert the unit attachment name
	assert_storage "dummy-storage-tp/0" "$(unit_attachment "data" 3 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "data" 3 "dummy-storage-tp" 0)"
	echo "Filesystem tmpfs PASSED"

	echo "Assessing filesystem"
	# assert the storage kind name
	assert_storage "filesystem" "$(kind_name "data" 4)"
	# assert the storage status
	assert_storage "alive" "$(life_status "data" 4)"
	# assert the storage label name
	assert_storage "data/4" "$(label 2)"
	# assert the unit attachment name
	assert_storage "dummy-storage-np/0" "$(unit_attachment "data" 4 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "data" 4 "dummy-storage-np" 0)"
	echo "Filesystem PASSED"

	echo "Assessing multiple filesystem, block, rootfs, loop"
	# assert the storage kind name
	assert_storage "filesystem" "$(kind_name "data" 5)"
	# assert the storage status
	assert_storage "alive" "$(life_status "data" 5)"
	# assert the storage label name
	assert_storage "data/5" "$(label 3)"
	# assert the unit attachment name
	assert_storage "dummy-storage-mp/0" "$(unit_attachment "data" 5 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "data" 5 "dummy-storage-mp" 0)"
	echo "Multiple filesystem, block, rootfs, loop PASSED"

	# remove the application
	juju remove-application dummy-storage-fs --destroy-storage
	juju remove-application dummy-storage-lp --destroy-storage
	juju remove-application dummy-storage-tp --destroy-storage
	juju remove-application dummy-storage-np --destroy-storage
	juju remove-application dummy-storage-mp --destroy-storage
	echo "All charm storage tests PASSED"

	destroy_model "${model_name}"
}

test_charm_storage() {
	if [ "$(skip 'test_charm_storage')" ]; then
		echo "==> TEST SKIPPED: charm storage tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_charm_storage"
	)
}
