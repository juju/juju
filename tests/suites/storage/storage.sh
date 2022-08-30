create_storage_pools() {
	echo

	model_name=$1

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

	if [ $? -eq 0 ]; then
		echo "Assessing storage pool PASSED"
	else
		echo "Assessing storage pool FAILED"
	fi
}

run_test_charm_storage() {
	echo

	model_name="test-charm-storage"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	create_storage_pools "${model_name}"

	echo "Assessing filesystem"
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage dummy-storage-np --series jammy --storage data=1G
	wait_for "dummy-storage-np" ".applications"
	# assert the storage kind name
	juju list-storage --format json | jq '.storage[].kind' | check "filesystem"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys | .[] ' | check "data/4"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage[].attachments.units | keys | .[] ' | check "dummy-storage-np/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[].attachments.units[].life' | check "alive"

	juju remove-application dummy-storage-np

	echo "Assessing block loop disk 1"
	juju deploy ./testcharms/charms/dummy-storage dummy-storage-lp --series="jammy" --storage disks=loop,1G
	wait_for "dummy-storage-lp" ".applications"
	# assert the storage kind name
	juju list-storage --format json | jq '.storage | .["disks/1"] | .kind' | check "block"
	# assert the storage label name
	juju list-storage --format json | jq '.storage| keys[1]' | check "disks/1"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage |.["disks/1"]|.attachments |.units | keys | .[]' | check "dummy-storage-lp/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[] | .attachments |.units[] | .life' | check "alive"

	echo "Assessing add storage block loop disk 2"
	juju add-storage -m "${model_name}" dummy-storage-lp/0 disks=1
	#	juju list-storage --format json | jq .
	# assert the storage kind name
	juju list-storage --format json | jq '.storage| .["disks/2"]| .kind' | check "block"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys[2]' | check "disks/2"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage | .["disks/2"] | .attachments |.units | keys | .[]' | check "dummy-storage-lp/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage | .["disks/2"] | .life' | check "alive"
	juju list-storage --format json | jq .

	juju remove-application dummy-storage-lp

	echo "Assessing filesystem rootfs"
	juju deploy ./testcharms/charms/dummy-storage dummy-storage-fs --series="jammy" --storage data=rootfs,1G
	wait_for "dummy-storage-fs" ".applications"
	# assert the storage kind name
	juju list-storage --format json | jq '.storage[].kind' | check "filesystem"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys | .[] ' | check "data/0"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage[].attachments.units | keys | .[] ' | check "dummy-storage-fs/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[].attachments.units[].life' | check "alive"

	juju remove-application dummy-storage-fs

	echo "Assessing filesystem tmpfs"
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage dummy-storage-tp --series jammy --storage multi-fs=tmpfs,1G
	wait_for "dummy-storage-tp" ".applications"
	juju list-storage --format json | jq .
	# assert the storage kind name
	juju list-storage --format json | jq '.storage[] | .kind' | check "filesystem"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys | .[] ' | check "multi-fs/3"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage[] | .attachments | .units | keys | .[] ' | check "dummy-storage-tp/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[] | .attachments | .units[] |.life' | check "alive"

	juju remove-application dummy-storage-tp
}

test_charm_storage() {

}
