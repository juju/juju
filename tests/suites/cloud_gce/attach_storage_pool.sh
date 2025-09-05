run_attach_storage_pool() {
	echo

	file="${TEST_DIR}/test-attach-storage-pool.log"

	ensure "test-attach-storage-pool" "${file}"

	juju create-storage-pool mygpd gce disk-type=pd-ssd
	juju deploy postgresql --channel 14/stable --storage pgdata=20G,mygpd
	wait_for_machine_agent_status "0" "started"

	disk_name="$(juju status --format=json | jq -r '.storage.volumes["0"]."provider-id"')"
	disk_info="$(gcloud compute disks list --filter="name=$disk_name" --format=json)"
	test 20 -eq $(jq -r '.[0].sizeGb' <<<"$disk_info")
	jq -r '.[0].type' <<<"$disk_info" | grep "/pd-ssd$"

	destroy_model "test-attach-storage-pool"
}

test_attach_storage_pool() {
	if [ "$(skip 'test_attach_storage_pool')" ]; then
		echo "==> TEST SKIPPED: attach storage pool"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_attach_storage_pool"
	)
}
