run_fs_storage() {
	echo

  model_name="test-fs-storage"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

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

	destroy_model "${model_name}"
}


test_fs_storage() {
	if [ "$(skip 'test_fs_storage')" ]; then
		echo "==> TEST SKIPPED: fs storage tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_fs_storage"
	)
}