test_reprovisioning() {
	if [ -n "$(skip 'test_reprovisioning')" ]; then
		echo "==> SKIP: Asked to skip test_reprovisioning tests"
		return
	fi

	case "${BOOTSTRAP_PROVIDER:-}" in
	"aws" | "ec2")
		check_dependencies juju aws yq
		;;
	*)
		echo "==> SKIP: Reprovisioning integration test requires AWS/EC2"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_reprovisioning_workload"
		run "run_reprovisioning_model_storage_rejection"
	)
}

run_reprovisioning_workload() {
	local file model_name model_uuid
	model_name="reprovisioning"
	file="${TEST_DIR}/test-${model_name}.log"

	ensure "${model_name}" "${file}"
	model_uuid=$(model_uuid "${model_name}")

	echo "Deploy a machine workload"
	juju deploy juju-qa-test reprovision
	wait_for "reprovision" "$(active_idle_condition "reprovision" 0)"

	local initial_status machine_id old_instance_id old_addresses
	local unit_names unit_machine
	initial_status=$(juju status --format json)
	machine_id=$(printf '%s\n' "${initial_status}" | yq -r '.applications.reprovision.units["reprovision/0"].machine')
	unit_names=$(printf '%s\n' "${initial_status}" | yq -r '.applications.reprovision.units | keys | sort | join(",")')
	unit_machine=$(printf '%s\n' "${initial_status}" | yq -r '.applications.reprovision.units["reprovision/0"].machine')

	local machine_info
	machine_info=$(juju show-machine "${machine_id}" --format json)
	old_instance_id=$(printf '%s\n' "${machine_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["instance-id"]')
	old_addresses=$(printf '%s\n' "${machine_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["ip-addresses"] | sort | join(",")')

	if [[ -z ${old_instance_id} || ${old_instance_id} == "null" ]]; then
		echo "ERROR: machine ${machine_id} has no provider instance ID" >&2
		return 1
	fi
	if [[ -z ${old_addresses} || ${old_addresses} == "null" ]]; then
		echo "ERROR: machine ${machine_id} has no provider addresses" >&2
		return 1
	fi

	echo "Stop the machine agent while leaving the provider instance running"
	juju ssh "${machine_id}" -- sudo systemctl stop "jujuagentd-machine-${machine_id}.service"
	wait_for_provider_running_refusal "${machine_id}"

	echo "Verify provider-running refusal did not mutate machine or unit identity"
	assert_machine_and_units "${machine_id}" "${old_instance_id}" "${old_addresses}" "${unit_names}" "${unit_machine}"

	echo "Externally terminate provider instance ${old_instance_id}"
	aws_ec2 terminate-instances --instance-ids "${old_instance_id}"
	aws_ec2 wait instance-terminated --instance-ids "${old_instance_id}"

	echo "Reprovision machine ${machine_id}"
	local output
	output=$(juju reprovision-machine "${machine_id}")
	check_contains "${output}" "reprovisioning machine ${machine_id}"
	check_contains "${output}" "root disk"
	check_contains "${output}" "ephemeral disk"
	check_contains "${output}" "charm-local state"
	check_contains "${output}" "machine-scoped storage data"

	local replacement_info new_instance_id new_addresses
	replacement_info=$(wait_for_replacement_machine "${machine_id}" "${old_instance_id}" "${old_addresses}")
	new_instance_id=$(printf '%s\n' "${replacement_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["instance-id"]')
	new_addresses=$(printf '%s\n' "${replacement_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["ip-addresses"] | sort | join(",")')

	wait_for "reprovision" "$(active_idle_condition "reprovision" 0)"
	assert_machine_and_units "${machine_id}" "${new_instance_id}" "${new_addresses}" "${unit_names}" "${unit_machine}"

	destroy_model "${model_name}"
	wait_for_ec2_instance_absent "${new_instance_id}"
	wait_for_ec2_model_resources_absent "${model_uuid}"
}

run_reprovisioning_model_storage_rejection() {
	local file model_name model_uuid
	model_name="reprovisioning-storage"
	file="${TEST_DIR}/test-${model_name}.log"

	ensure "${model_name}" "${file}"
	model_uuid=$(model_uuid "${model_name}")

	echo "Deploy a workload with model-scoped EBS storage"
	juju deploy juju-qa-dummy-storage reprovision-storage \
		--storage single-blk=ebs,1,1G
	wait_for "reprovision-storage" "$(active_idle_condition "reprovision-storage" 0)"

	local status machine_id instance_id volume_id unit_names unit_machine output
	status=$(juju status --format json)
	machine_id=$(printf '%s\n' "${status}" | yq -r '.applications."reprovision-storage".units["reprovision-storage/0"].machine')
	unit_names=$(printf '%s\n' "${status}" | yq -r '.applications."reprovision-storage".units | keys | sort | join(",")')
	unit_machine=$(printf '%s\n' "${status}" | yq -r '.applications."reprovision-storage".units["reprovision-storage/0"].machine')
	instance_id=$(juju show-machine "${machine_id}" --format json | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["instance-id"]')
	volume_id=$(juju storage --format json | yq -r '.volumes["0"]["provider-id"]')

	if output=$(juju reprovision-machine "${machine_id}" 2>&1); then
		echo "ERROR: reprovisioning unexpectedly accepted model-scoped storage" >&2
		return 1
	fi
	check_contains "${output}" "model-scoped storage attached to machine"

	local current_instance_id current_volume_id current_unit_names current_unit_machine
	current_instance_id=$(juju show-machine "${machine_id}" --format json | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["instance-id"]')
	current_volume_id=$(juju storage --format json | yq -r '.volumes["0"]["provider-id"]')
	status=$(juju status --format json)
	current_unit_names=$(printf '%s\n' "${status}" | yq -r '.applications."reprovision-storage".units | keys | sort | join(",")')
	current_unit_machine=$(printf '%s\n' "${status}" | yq -r '.applications."reprovision-storage".units["reprovision-storage/0"].machine')
	if [[ ${current_instance_id} != "${instance_id}" ]]; then
		echo "ERROR: model-scoped storage refusal changed the provider instance" >&2
		return 1
	fi
	if [[ ${current_volume_id} != "${volume_id}" ]]; then
		echo "ERROR: model-scoped storage refusal changed the volume" >&2
		return 1
	fi
	if [[ ${current_unit_names} != "${unit_names}" || ${current_unit_machine} != "${unit_machine}" ]]; then
		echo "ERROR: model-scoped storage refusal changed unit identity or assignment" >&2
		return 1
	fi

	destroy_model "${model_name}"
	wait_for_ec2_instance_absent "${instance_id}"
	wait_for_ebs_volume_absent "${volume_id}"
	wait_for_ec2_model_resources_absent "${model_uuid}"
}

wait_for_provider_running_refusal() {
	local machine_id=$1
	local output start_time elapsed
	start_time=$(date -u +%s)

	while true; do
		if output=$(juju reprovision-machine "${machine_id}" 2>&1); then
			echo "ERROR: reprovisioning accepted a running provider instance" >&2
			return 1
		fi
		if [[ ${output} == *"machine provider instance is running"* ]]; then
			return
		fi
		if [[ ${output} != *"machine agent is still present"* ]]; then
			echo "ERROR: unexpected reprovisioning refusal: ${output}" >&2
			return 1
		fi

		sleep "${SHORT_TIMEOUT}"
		elapsed=$(($(date -u +%s) - start_time))
		if [[ ${elapsed} -ge 600 ]]; then
			echo "ERROR: timed out waiting for provider-running refusal" >&2
			return 1
		fi
	done
}

wait_for_replacement_machine() {
	local machine_id=$1
	local old_instance_id=$2
	local old_addresses=$3
	local machine_info instance_id addresses agent_status start_time elapsed
	start_time=$(date -u +%s)

	while true; do
		machine_info=$(juju show-machine "${machine_id}" --format json 2>/dev/null || true)
		instance_id=$(printf '%s\n' "${machine_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["instance-id"] // ""')
		addresses=$(printf '%s\n' "${machine_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["ip-addresses"] // [] | sort | join(",")')
		agent_status=$(printf '%s\n' "${machine_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["juju-status"].current // ""')
		if [[ -n ${instance_id} && ${instance_id} != "${old_instance_id}" &&
			-n ${addresses} && ${addresses} != "${old_addresses}" &&
			${agent_status} == "started" ]]; then
			printf '%s\n' "${machine_info}"
			return
		fi

		sleep "${SHORT_TIMEOUT}"
		elapsed=$(($(date -u +%s) - start_time))
		if [[ ${elapsed} -ge 900 ]]; then
			echo "ERROR: timed out waiting for replacement machine ${machine_id}" >&2
			juju show-machine "${machine_id}" >&2 || true
			return 1
		fi
	done
}

assert_machine_and_units() {
	local machine_id=$1
	local expected_instance_id=$2
	local expected_addresses=$3
	local expected_unit_names=$4
	local expected_unit_machine=$5
	local machine_info status instance_id addresses unit_names unit_machine

	machine_info=$(juju show-machine "${machine_id}" --format json)
	instance_id=$(printf '%s\n' "${machine_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["instance-id"]')
	addresses=$(printf '%s\n' "${machine_info}" | machine_id="${machine_id}" yq -r '.machines[env(machine_id)]["ip-addresses"] | sort | join(",")')
	status=$(juju status --format json)
	unit_names=$(printf '%s\n' "${status}" | yq -r '.applications.reprovision.units | keys | sort | join(",")')
	unit_machine=$(printf '%s\n' "${status}" | yq -r '.applications.reprovision.units["reprovision/0"].machine')

	if [[ ${instance_id} != "${expected_instance_id}" || ${addresses} != "${expected_addresses}" ]]; then
		echo "ERROR: machine ${machine_id} provider data changed unexpectedly" >&2
		return 1
	fi
	if [[ ${unit_names} != "${expected_unit_names}" || ${unit_machine} != "${expected_unit_machine}" ]]; then
		echo "ERROR: unit identity or assignment changed unexpectedly" >&2
		return 1
	fi
}

aws_ec2() {
	local region
	region=$(aws_region)
	if [[ -n ${region} && ${region} != "null" ]]; then
		aws ec2 "$@" --region "${region}"
		return
	fi
	aws ec2 "$@"
}

aws_region() {
	if [[ -n ${AWS_REGION:-} ]]; then
		printf '%s\n' "${AWS_REGION}"
		return
	fi
	juju show-model --format json | yq -r 'to_entries[0].value.region // ""'
}

model_uuid() {
	local model_name=$1
	juju show-model "${model_name}" --format json |
		yq -r 'to_entries[0].value["model-uuid"]'
}

wait_for_ec2_model_resources_absent() {
	local model_uuid=$1
	local volumes security_groups start_time elapsed
	start_time=$(date -u +%s)

	while true; do
		volumes=$(aws_ec2 describe-volumes \
			--filters "Name=tag:juju-model-uuid,Values=${model_uuid}" \
			--query 'Volumes[].VolumeId' --output text 2>/dev/null || true)
		security_groups=$(aws_ec2 describe-security-groups \
			--filters "Name=tag:juju-model-uuid,Values=${model_uuid}" \
			--query 'SecurityGroups[].GroupId' --output text 2>/dev/null || true)
		if [[ -z ${volumes} && -z ${security_groups} ]]; then
			return
		fi

		sleep "${SHORT_TIMEOUT}"
		elapsed=$(($(date -u +%s) - start_time))
		if [[ ${elapsed} -ge 600 ]]; then
			echo "ERROR: leaked EC2 model resources for ${model_uuid}: volumes=${volumes} security-groups=${security_groups}" >&2
			return 1
		fi
	done
}

wait_for_ec2_instance_absent() {
	local instance_id=$1
	local state start_time elapsed
	start_time=$(date -u +%s)

	while true; do
		state=$(aws_ec2 describe-instances --instance-ids "${instance_id}" \
			--query 'Reservations[0].Instances[0].State.Name' --output text 2>/dev/null || true)
		if [[ -z ${state} || ${state} == "None" || ${state} == "terminated" ]]; then
			return
		fi

		sleep "${SHORT_TIMEOUT}"
		elapsed=$(($(date -u +%s) - start_time))
		if [[ ${elapsed} -ge 600 ]]; then
			echo "ERROR: leaked EC2 instance ${instance_id} in state ${state}" >&2
			return 1
		fi
	done
}

wait_for_ebs_volume_absent() {
	local volume_id=$1
	local state start_time elapsed
	start_time=$(date -u +%s)

	while true; do
		state=$(aws_ec2 describe-volumes --volume-ids "${volume_id}" \
			--query 'Volumes[0].State' --output text 2>/dev/null || true)
		if [[ -z ${state} || ${state} == "None" || ${state} == "deleted" ]]; then
			return
		fi

		sleep "${SHORT_TIMEOUT}"
		elapsed=$(($(date -u +%s) - start_time))
		if [[ ${elapsed} -ge 600 ]]; then
			echo "ERROR: leaked EBS volume ${volume_id} in state ${state}" >&2
			return 1
		fi
	done
}
