# This subtest tests that juju can create storage pools for the different storage providers
# and can deploy a charm and make use of the already provisioned different storage types.
run_charm_storage() {
	echo

	model_name="charm-storage"
	file="${TEST_DIR}/test-${model_name}.log"

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
		juju list-storage-pools -m "${model_name}" --format json | jq '.ebs | .provider' | check "ebs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.["ebs-ssd"] | .provider' | check "ebs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.tmpfs | .provider' | check "tmpfs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.loop | .provider' | check "loop"
		juju list-storage-pools -m "${model_name}" --format json | jq '.rootfs | .provider' | check "rootfs"
	else
		juju list-storage-pools -m "${model_name}" --format json | jq '.rooty | .provider' | check "rootfs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.tempy | .provider' | check "tmpfs"
		juju list-storage-pools -m "${model_name}" --format json | jq '.loopy | .provider' | check "loop"
		juju list-storage-pools -m "${model_name}" --format json | jq '.ebsy | .provider' | check "ebs"
	fi
	echo "Storage pool PASSED"

	# Assess charm storage with the filesystem storage provider
	echo "Assessing filesystem rootfs"
	juju deploy ./testcharms/charms/dummy-storage-fs --series="jammy" --storage data=rootfs,1G
	wait_for "dummy-storage-fs" ".applications"
	# assert the storage kind name
	juju list-storage --format json | jq '.storage[] | .kind' | check "filesystem"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys | .[]' | check "data/0"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage[] | .attachments | .units | keys | .[]' | check "dummy-storage-fs/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[] | .attachments | .units[] | .life' | check "alive"
	echo "Filesystem rootfs PASSED"
	# remove the application
	juju remove-application dummy-storage-fs
	wait_for "{}" ".applications"
	# Assess charm storage with the filesystem storage provider
	echo "Assessing block loop disk 1"
	juju deploy ./testcharms/charms/dummy-storage-lp --series="jammy" --storage disks=loop,1G
	wait_for "dummy-storage-lp" ".applications"
	# assert the storage kind name
	juju list-storage --format json | jq '.storage | .["disks/1"] | .kind' | check "block"
	# assert the storage label name
	juju list-storage --format json | jq '.storage| keys[0]' | check "disks/1"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage |.["disks/1"]|.attachments |.units | keys | .[]' | check "dummy-storage-lp/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[] | .attachments |.units[] | .life' | check "alive"
	echo "Block loop disk 1 PASSED"
	echo "Assessing add storage block loop disk 2"
	juju add-storage -m "${model_name}" dummy-storage-lp/0 disks=1
	# assert the storage kind name
	juju list-storage --format json | jq '.storage| .["disks/2"]| .kind' | check "block"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys[1]' | check "disks/2"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage | .["disks/2"] | .attachments |.units | keys | .[]' | check "dummy-storage-lp/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage | .["disks/2"] | .life' | check "alive"
	echo "Block loop disk 2 PASSED"
	# remove the application
	juju remove-application dummy-storage-lp
	wait_for "{}" ".applications"
	echo "Assessing filesystem tmpfs"
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-tp --series jammy --storage data=tmpfs,1G
	wait_for "dummy-storage-tp" ".applications"
	# assert the storage kind name
	juju list-storage --format json | jq '.storage[] | .kind' | check "filesystem"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys | .[] ' | check "data/3"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage[] | .attachments | .units | keys | .[] ' | check "dummy-storage-tp/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[] | .attachments | .units[] |.life' | check "alive"
	echo "Filesystem tmpfs PASSED"
	# remove the application
	juju remove-application dummy-storage-tp
	wait_for "{}" ".applications"
	echo "Assessing filesystem"
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-np --series jammy --storage data=1G
	wait_for "dummy-storage-np" ".applications"
	# assert the storage kind name
	juju list-storage --format json | jq '.storage[] | .kind' | check "filesystem"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys | .[]' | check "data/4"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage[] | .attachments | .units | keys | .[]' | check "dummy-storage-np/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[] | .attachments | .units[] | .life' | check "alive"
	echo "Filesystem PASSED"
	# remove application
	juju remove-application dummy-storage-np
	wait_for "{}" ".applications"
	# We remove storage data/4 since in Juju 2.3+ it is persistent. Otherwise it will interfere with the next test's results
	juju remove-storage data/4
	echo "Assessing multiple filesystem, block, rootfs, loop"
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-mp --series jammy --storage data=1G
	wait_for "dummy-storage-mp" ".applications"
	# assert the storage kind name
	juju list-storage --format json | jq '.storage | .["data/5"] | .kind' | check "filesystem"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys | .[0]' | check "data/5"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage | .["data/5"] | .attachments | .units | keys | .[]' | check "dummy-storage-mp/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage | .["data/5"]  | .attachments | .units[] | .life' | check "alive"
	echo "Multiple filesystem, block, rootfs, loop PASSED"
	# remove application
	juju remove-application dummy-storage-mp
	wait_for "{}" ".applications"
	echo "All charm storage tests PASSED"

	destroy_model "${model_name}"
}

test_charm_storage() {
	if [ "$(skip 'test-charm-storage')" ]; then
		echo "==> TEST SKIPPED: charm storage tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_charm_storage"
	)
}
