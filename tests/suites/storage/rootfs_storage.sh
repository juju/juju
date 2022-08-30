run_rootfs_storage() {
	echo

  model_name="test-rootfs-storage"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

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

	destroy_model "${model_name}"
}

test_rootfs_storage() {
	if [ "$(skip 'test_rootfs_storage')" ]; then
		echo "==> TEST SKIPPED: filesystem rootfs tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_rootfs_storage"
	)
}