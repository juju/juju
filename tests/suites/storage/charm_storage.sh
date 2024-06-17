# This subtest tests that juju can create storage pools for the different storage providers
# and can deploy a charm and make use of the already provisioned different storage types.
run_charm_storage() {
	echo

	model_name="charm-storage"
	file="${TEST_DIR}/test-${model_name}.log"

	ensure "${model_name}" "${file}"

	echo "Assessing default storage pools"
	juju list-storage-pools -m "${model_name}" --format json | jq '.loop | .provider' | check "loop"
	juju list-storage-pools -m "${model_name}" --format json | jq '.rootfs | .provider' | check "rootfs"
	juju list-storage-pools -m "${model_name}" --format json | jq '.tmpfs | .provider' | check "tmpfs"
	if [ "${BOOTSTRAP_PROVIDER:-}" == "ec2" ]; then
		juju list-storage-pools -m "${model_name}" --format json | jq '.["ebs-ssd"] | .provider' | check "ebs"
	fi
	echo "Default storage pools PASSED"

	echo "Assess create-storage-pool"
	juju create-storage-pool -m "${model_name}" loopy loop size=1G
	juju create-storage-pool -m "${model_name}" rooty rootfs size=1G
	juju create-storage-pool -m "${model_name}" tempy tmpfs size=1G
	juju create-storage-pool -m "${model_name}" ebsy ebs size=1G
	echo "create-storage-pool PASSED"

	# Assess the above created storage pools.
	echo "Assessing storage pool"
	juju list-storage-pools -m "${model_name}" --format json | jq '.rooty | .provider' | check "rootfs"
	juju list-storage-pools -m "${model_name}" --format json | jq '.tempy | .provider' | check "tmpfs"
	juju list-storage-pools -m "${model_name}" --format json | jq '.loopy | .provider' | check "loop"
	juju list-storage-pools -m "${model_name}" --format json | jq '.ebsy | .provider' | check "ebs"
	echo "Storage pool PASSED"

	# Assess charm storage with the filesystem storage provider
	echo "Assessing filesystem rootfs"
	juju deploy ./testcharms/charms/dummy-storage-fs --base ubuntu@22.04 --storage data=rootfs,1G
	wait_for "dummy-storage-fs" ".applications"
	if [ "$(unit_exist "data/0")" == "true" ]; then
		assess_rootfs
	fi
	# remove the application
	juju remove-application --no-prompt dummy-storage-fs
	wait_for "{}" ".applications"

	# Assess charm storage with the filesystem storage provider
	echo "Assessing block loop disk 1"
	juju deploy ./testcharms/charms/dummy-storage-lp --base ubuntu@22.04 --storage disks=loop,1G
	wait_for "dummy-storage-lp" ".applications"
	# assert the storage kind name
	if [ "$(unit_exist "disks/1")" == "true" ]; then
		assess_loop_disk1
	fi

	#Assessing adding a storage block to loop disk
	juju add-storage -m "${model_name}" dummy-storage-lp/0 disks=1
	# assert the storage kind name
	if [ "$(unit_exist "disks/2")" == "true" ]; then
		assess_loop_disk2
	fi
	# remove the application
	juju remove-application --no-prompt dummy-storage-lp
	wait_for "{}" ".applications"

	# Assess tmpfs pool for the filesystem provider
	echo "Assessing filesystem tmpfs"
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-tp --base ubuntu@22.04 --storage data=tmpfs,1G
	wait_for "dummy-storage-tp" ".applications"
	if [ "$(unit_exist "data/3")" == "true" ]; then
		assess_tmpfs
	fi
	# remove the application
	juju remove-application --no-prompt dummy-storage-tp
	wait_for "{}" ".applications"

	#Assessing for persistent filesystem
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-np --base ubuntu@22.04 --storage data=1G
	wait_for "dummy-storage-np" ".applications"
	if [ "$(unit_exist "data/4")" == "true" ]; then
		assess_fs
	fi
	# remove application
	juju remove-application --no-prompt dummy-storage-np
	wait_for "{}" ".applications"
	# We remove storage data/4 since in Juju 2.3+ it is persistent. Otherwise it will interfere with the next test's results
	juju remove-storage data/4

	#Assessing multiple filesystem, block, rootfs, loop"
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-mp --base ubuntu@22.04 --storage data=1G
	wait_for "dummy-storage-mp" ".applications"
	if [ "$(unit_exist "data/5")" == "true" ]; then
		assess_multiple_fs
	fi
	# remove application
	juju remove-application --no-prompt dummy-storage-mp
	wait_for "{}" ".applications"
	echo "All charm storage tests PASSED"

	destroy_model "${model_name}"
}

assess_rootfs() {
	echo "Assessing filesystem rootfs"
	# Assess charm storage with the filesystem storage provider
	wait_for_storage "attached" '.storage["data/0"]["status"].current'
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
}

assess_loop_disk1() {
	# Assess charm storage with the filesystem storage provider
	echo "Assessing block loop disk 1"
	wait_for_storage "attached" '.storage["disks/1"]["status"].current'
	# assert the storage kind name
	assert_storage "block" "$(kind_name "disks" 1)"
	# assert the storage status
	assert_storage "alive" "$(life_status "disks" 1)"
	# assert the storage label name
	assert_storage "disks/1" "$(label 0)"
	# assert the unit attachment name
	assert_storage "dummy-storage-lp/0" "$(unit_attachment "disks" 1 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "disks" 1 "dummy-storage-lp" 0)"
	echo "Block loop disk 1 PASSED"
}

assess_loop_disk2() {
	echo "Assessing add storage block loop disk 2"
	wait_for_storage "attached" '.storage["disks/2"]["status"].current'
	# assert the storage kind name
	assert_storage "block" "$(kind_name "disks" 2)"
	# assert the storage status
	assert_storage "alive" "$(life_status "disks" 2)"
	# assert the storage label name
	assert_storage "disks/2" "$(label 1)"
	# assert the unit attachment name
	assert_storage "dummy-storage-lp/0" "$(unit_attachment "disks" 2 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "disks" 2 "dummy-storage-lp" 0)"
	echo "Block loop disk 2 PASSED"
}

assess_tmpfs() {
	echo "Assessing filesystem tmpfs"
	wait_for_storage "attached" '.storage["data/3"]["status"].current'
	# assert the storage kind name
	assert_storage "filesystem" "$(kind_name "data" 3)"
	# assert the storage status
	assert_storage "alive" "$(life_status "data" 3)"
	# assert the storage label name
	assert_storage "data/3" "$(label 0)"
	# assert the unit attachment name
	assert_storage "dummy-storage-tp/0" "$(unit_attachment "data" 3 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "data" 3 "dummy-storage-tp" 0)"
	echo "Filesystem tmpfs PASSED"
}

assess_fs() {
	echo "Assessing filesystem"
	wait_for_storage "attached" '.storage["data/4"]["status"].current'
	# assert the storage kind name
	assert_storage "filesystem" "$(kind_name "data" 4)"
	# assert the storage status
	assert_storage "alive" "$(life_status "data" 4)"
	# assert the storage label name
	assert_storage "data/4" "$(label 0)"
	# assert the unit attachment name
	assert_storage "dummy-storage-np/0" "$(unit_attachment "data" 4 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "data" 4 "dummy-storage-np" 0)"
	echo "Filesystem PASSED"
}

assess_multiple_fs() {
	echo "Assessing multiple filesystem, block, rootfs, loop"
	wait_for_storage "attached" '.storage["data/5"]["status"].current'
	# assert the storage kind name
	assert_storage "filesystem" "$(kind_name "data" 5)"
	# assert the storage status
	assert_storage "alive" "$(life_status "data" 5)"
	# assert the storage label name
	assert_storage "data/5" "$(label 0)"
	# assert the unit attachment name
	assert_storage "dummy-storage-mp/0" "$(unit_attachment "data" 5 0)"
	# assert the attached unit state
	assert_storage "alive" "$(unit_state "data" 5 "dummy-storage-mp" 0)"
	echo "Multiple filesystem, block, rootfs, loop PASSED"
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
