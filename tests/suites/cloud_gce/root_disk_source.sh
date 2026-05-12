get_machine_message() {
	local machine_id=$1

	juju status --format=yaml | yq -r ".machines[\"${machine_id}\"][\"machine-status\"].message"
}

deploy_root_disk_source_app() {
	local app_name=$1
	local constraints=${2:-}

	if [ -n "${constraints}" ]; then
		juju deploy juju-qa-test "${app_name}" --channel latest/edge --constraints "${constraints}"
	else
		juju deploy juju-qa-test "${app_name}" --channel latest/edge
	fi
}

# Assert that deploying an application with valid root disk source succeeds.
assert_root_disk_source_succeed() {
	local app_name=$1
	local expected_disk_type=$2
	local constraints=${3:-}
	local machine_id
	local instance_id
	local az
	local boot_disk_type

	echo "Deploying ${app_name} with constraints: ${constraints:-<none>}"
	deploy_root_disk_source_app "${app_name}" "${constraints}"

	machine_id="$(juju status --format=yaml | yq -r ".applications[\"${app_name}\"].units[\"${app_name}/0\"].machine")"
	wait_for_machine_agent_status "${machine_id}" "started"

	instance_id="$(juju show-machine "${machine_id}" --format=yaml | yq -r ".machines[\"${machine_id}\"][\"instance-id\"]")"
	az="$(juju show-machine "${machine_id}" --format=yaml | yq -r ".machines[\"${machine_id}\"].hardware" | tr ' ' '\n' | grep 'availability-zone' | cut -d= -f2)"
	boot_disk_type="$(gcloud compute disks describe "${instance_id}" --zone="${az}" --format="value(type.basename())")"
	if [ "${boot_disk_type}" != "${expected_disk_type}" ]; then
		echo "FAIL: expected boot disk type ${expected_disk_type} for ${app_name}, got ${boot_disk_type}"
		return 1
	fi

	echo "OK: ${app_name} boot disk type is ${expected_disk_type}"
}

# Assert that deploying an application with invalid root disk source fails with expected error message.
assert_root_disk_source_failure() {
	local app_name=$1
	local constraints=$2
	local expected_message=$3
	local machine_id
	local machine_msg

	echo "Deploying ${app_name} with constraints: ${constraints}"
	deploy_root_disk_source_app "${app_name}" "${constraints}"

	machine_id="$(juju status --format=yaml | yq -r ".applications[\"${app_name}\"].units[\"${app_name}/0\"].machine")"
	echo "Waiting for failure message for ${app_name} on machine ${machine_id}..."
	if wait_for "${expected_message}" ".machines[\"${machine_id}\"][\"machine-status\"][\"message\"]"; then
		machine_msg="$(get_machine_message "${machine_id}")"
		echo "OK: ${app_name} correctly rejected with message: ${machine_msg}"
		return 0
	fi

	machine_msg="$(get_machine_message "${machine_id}")"
	echo "FAIL: expected ${expected_message} in machine status for ${app_name}, got: ${machine_msg}"
	return 1
}

run_root_disk_source() {
	root_disk_source_model_name="test-root-disk-source"

	file="${TEST_DIR}/test-root-disk-source.log"
	ensure "${root_disk_source_model_name}" "${file}"

	# Run deploy with no root disk source specified, expect default disk type to be used.
	assert_root_disk_source_succeed "default" "pd-standard"
	# Run deploy with root disk source specified as disk type, expect specified disk type to be used.
	assert_root_disk_source_succeed "disk-type-ssd" "pd-ssd" "root-disk-source=pd-ssd"
	assert_root_disk_source_failure "disk-type-local" "root-disk-source=local-ssd" "local SSD disk storage not valid"
	assert_root_disk_source_failure "disk-type-invalid" "root-disk-source=invalid-disk" 'root disk source ".*" not valid'

	# Create storage pools with different disk types and run deploy with root disk source specified as storage pool, expect disk type of storage pool to be used.
	juju create-storage-pool ssd-gce gce disk-type=pd-ssd
	juju create-storage-pool local-ssd gce disk-type=pd-ssd
	juju create-storage-pool local-gce gce disk-type=local-ssd
	juju create-storage-pool invalid-disk gce disk-type=invalid-disk
	assert_root_disk_source_succeed "storage-pool-pd-ssd" "pd-ssd" "root-disk-source=ssd-gce"
	assert_root_disk_source_succeed "storage-pool-local-ssd-name" "pd-ssd" "root-disk-source=local-ssd"
	assert_root_disk_source_failure "storage-pool-local-ssd" "root-disk-source=local-gce" "local SSD disk storage not valid"
	assert_root_disk_source_failure "storage-pool-invalid" "root-disk-source=invalid-disk" 'disk type ".*" for root disk not valid'

	destroy_model "${root_disk_source_model_name}"
}

test_root_disk_source() {
	if [ "$(skip 'test_root_disk_source')" ]; then
		echo "==> TEST SKIPPED: root disk source"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_root_disk_source"
	)
}
