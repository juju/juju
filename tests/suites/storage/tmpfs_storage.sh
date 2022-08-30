run_tmpfs_storage() {
	echo

  model_name="test-charm-storage"
	file="${TEST_DIR}/${model_name}.log"


	ensure "${model_name}" "${file}"

	create_storage_pools "${model_name}"

	echo "Assessing filesystem tmpfs"
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage dummy-storage-tp --series jammy --storage data=tmpfs,1G
	wait_for "dummy-storage-tp" ".applications"
  juju list-storage --format json | jq .
	# assert the storage kind name
	juju list-storage --format json | jq '.storage[] | .kind' | check "filesystem"
	# assert the storage label name
	juju list-storage --format json | jq '.storage | keys | .[] ' | check "data/3"
	# assert the unit attachment name
	juju list-storage --format json | jq '.storage[] | .attachments | .units | keys | .[] ' | check "dummy-storage-tp/0"
	# assert the attached unit state
	juju list-storage --format json | jq '.storage[] | .attachments | .units[] |.life' | check "alive"

	juju remove-application dummy-storage-tp

	destroy_model "${model_name}"
}

test_tmpfs_storage() {
	if [ "$(skip 'test-tmpfs-storage')" ]; then
		echo "==> TEST SKIPPED: tmpfs storage tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_tmpfs_storage"
	)
}