run_block_storage() {
	echo

 	model_name="test-block-storage"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

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

	destroy_model "${model_name}"
}


test_block_storage() {
	if [ "$(skip 'test-block-storage')" ]; then
		echo "==> TEST SKIPPED: block storage tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_block_storage"
	)
}