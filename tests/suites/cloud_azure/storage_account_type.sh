check_storage_account_type_controller() {
	local controller_name="${1}"
	local constraints="${2}"
	local controller_acct_type_expected="${3:-StandardSSD_LRS}"
	local file="${TEST_DIR}/test-${controller_name}.log"

	cloud="azure"
	if [[ -n ${BOOTSTRAP_REGION:-} ]]; then
		cloud_region="${cloud}/${BOOTSTRAP_REGION}"
	else
		cloud_region="${cloud}"
	fi

	bs_cmd=(juju bootstrap "${cloud_region}" "${controller_name}" --show-log)
	if [[ -n ${constraints} ]]; then
		# Split raw flag into string and append each element to bs_cmd.
		read -r -a constraint_args <<<"$constraints"
		bs_cmd+=("${constraint_args[@]}")
	fi

	if ! "${bs_cmd[@]}" >>"${file}" 2>&1; then
		echo "Bootstrap failed for ${controller_name}"
		return 1
	fi
	# This will clean up the controller later.
	echo "${controller_name}" >>"${TEST_DIR}/jujus" || return 1

	juju add-model test || return 1

	# Validate controller VM's storageAccountType.
	local controller_instance_id controller_resource_group
	controller_instance_id=$(juju show-controller "${controller_name}" --format yaml 2>/dev/null | yq -r ".${controller_name}.\"controller-machines\"[] .instance-id" 2>/dev/null || true)
	if [[ -z ${controller_instance_id} || ${controller_instance_id} == "null" || ${controller_instance_id} == "pending" ]]; then
		echo "ERROR: could not determine controller instance-id for ${controller_name}" >>"${file}"
		return 1
	else
		controller_resource_group=$(az vm list -o yaml 2>/dev/null | yq -r ".[] | select(.name == \"${controller_instance_id}\") | .resourceGroup" 2>/dev/null || true)
		if [[ -z ${controller_resource_group} || ${controller_resource_group} == "null" ]]; then
			echo "ERROR: could not find resource group for controller instance-id ${controller_instance_id}" >>"${file}"
			return 1
		else
			controller_acct_type=$(az vm show -g "${controller_resource_group}" -n "${controller_instance_id}" --query "storageProfile.osDisk.managedDisk.storageAccountType" -o tsv 2>/dev/null || true)
			if [[ ${controller_acct_type} != "${controller_acct_type_expected}" ]]; then
				echo "MISMATCH: controller ${controller_name} expected storageAccountType ${controller_acct_type_expected} but got ${controller_acct_type}" >>"${file}"
				return 1
			else
				echo "OK: controller ${controller_name} storageAccountType matches expected value ${controller_acct_type_expected}" >>"${file}"
			fi
		fi
	fi

	# Use optional test storage, without PostgreSQL's Juju version restriction.
	# Run the deploy steps for all cases.
	echo "Deploying ps1..ps4" >>"${file}"
	(
		set -o pipefail
		# Default azure account type with no additional charm storage.
		juju deploy juju-qa-dummy-storage ps1 --channel latest/stable --revision 1 --base ubuntu@24.04 2>&1 | OUTPUT "${file}" || exit 1
		# Premium azure account type with no additional charm storage.
		juju deploy juju-qa-dummy-storage ps2 --channel latest/stable --revision 1 --base ubuntu@24.04 --constraints "instance-type=Standard_D8as_v5 root-disk-source=azure-premium" 2>&1 | OUTPUT "${file}" || exit 1
		# Default azure account type with additional premium charm storage.
		juju deploy juju-qa-dummy-storage ps3 --channel latest/stable --revision 1 --base ubuntu@24.04 --constraints "instance-type=Standard_D8as_v5" --storage single-fs=2G,azure-premium 2>&1 | OUTPUT "${file}" || exit 1
		# Premium azure account type with additional default charm storage.
		juju deploy juju-qa-dummy-storage ps4 --channel latest/stable --revision 1 --base ubuntu@24.04 --constraints "instance-type=Standard_D8as_v5 root-disk-source=azure-premium" --storage single-fs=2G,azure 2>&1 | OUTPUT "${file}" || exit 1
	) || return 1

	# Wait for Juju to register machines for the deployed units and provide instance-ids.
	echo "Waiting up to 5min for instance-ids for ps1..ps4" >>"${file}"
	timeout=300
	interval=5
	elapsed=0

	# Array of apps to check, and their expected storageAccountType for OS disk and charm disk.
	# Empty charm storageAccountType means no charm storage to validate.
	apps=(ps1 ps2 ps3 ps4)
	disk_os_storage_acct_types=(StandardSSD_LRS Premium_LRS StandardSSD_LRS Premium_LRS)
	charm_storage_acct_types=("" "" Premium_LRS StandardSSD_LRS)

	# Wait for all apps to have instance-ids assigned.
	# This would indicate that the VMs and storage have been provisioned.
	while [[ $elapsed -lt $timeout ]]; do
		STATUS_YAML=$(juju status --format=json 2>/dev/null || true)
		missing=0
		for app in "${apps[@]}"; do
			machine=$(echo "${STATUS_YAML}" | yq e -r ".applications.${app}.units[] | .machine" - 2>/dev/null || echo "")
			if [[ -z ${machine} ]]; then
				missing=1
				break
			fi
			iid=$(echo "${STATUS_YAML}" | yq e -r ".machines[\"${machine}\"].instance-id" - 2>/dev/null || echo "")
			if [[ -z ${iid} || ${iid} == "null" || ${iid} == "pending" ]]; then
				missing=1
				break
			fi
		done
		# All apps have instance-ids.
		if [[ ${missing} -eq 0 ]]; then
			break
		fi
		sleep ${interval}
		elapsed=$((elapsed + interval))
	done
	if [[ ${elapsed} -ge ${timeout} ]]; then
		echo "ERROR: timeout waiting for instance-ids for apps: ${apps[*]}" >>"${file}"
		return 1
	fi

	# For each app, get the unit's instance-id, map to AZ resource group, and query storageAccountType.
	for i in "${!apps[@]}"; do
		app=${apps[$i]}
		STATUS_YAML=$(juju status --format=yaml 2>/dev/null || true)

		# Unit is expected to be <app>/0 (e.g. ps1/0).
		unit="${app}/0"

		# Retrieve the machine id.
		machine=$(echo "${STATUS_YAML}" | yq e -r ".applications.${app}.units.${unit}.machine" - 2>/dev/null)
		if [[ -z ${machine} || ${machine} == "null" ]]; then
			echo "ERROR: no machine for ${app} unit ${unit}" >>"${file}"
			return 1
		fi

		# Retrieve the instance id.
		iid=$(echo "${STATUS_YAML}" | yq e -r ".machines.${machine}.instance-id" - 2>/dev/null)
		if [[ -z ${iid} || ${iid} == "null" || ${iid} == "pending" ]]; then
			echo "ERROR: no instance-id for ${app} (machine ${machine})" >>"${file}"
			return 1
		fi

		# Retrieve the resource group using the instance id.
		rg=$(az vm list -o yaml 2>/dev/null | yq -r ".[] | select(.name == \"${iid}\") | .resourceGroup" 2>/dev/null || true)
		if [[ -z ${rg} || ${rg} == "null" ]]; then
			echo "ERROR: could not find resource group for instance-id ${iid}" >>"${file}"
			return 1
		fi

		# Retrieve the storageAccountType using the resource group and instance-id.
		acct_type=$(az vm show -g "${rg}" -n "${iid}" --query "storageProfile.osDisk.managedDisk.storageAccountType" -o tsv 2>/dev/null || true)
		echo "storageAccountType for ${iid} (rg=${rg}): ${acct_type}" >>"${file}"

		# Validate actual storageAccountType against expected storageAccountType for OS disk.
		expected="${disk_os_storage_acct_types[$i]:-}"
		if [[ ${acct_type} != "${expected}" ]]; then
			echo "MISMATCH: app ${app} expected ${expected} but got ${acct_type}" >>"${file}"
			return 1
		else
			echo "OK: app ${app} storageAccountType matches expected value ${expected}" >>"${file}"
		fi

		# Iterate to next app since there is no charm storage to validate for this app.
		if [[ ${charm_storage_acct_types[$i]} == "" ]]; then
			continue
		fi

		# Retrieve the provider-id (volume name) attached to the app unit, eg. volume-2.
		volume=$(juju storage --format=yaml 2>/dev/null | yq -r ".volumes[] | select(.attachments.units.\"${unit}\") | .\"provider-id\"" 2>/dev/null || true)
		if [[ -z ${volume} || ${volume} == "null" ]]; then
			echo "ERROR: no charm volume found for ${app} unit ${unit}" >>"${file}"
			return 1
		fi

		# Validate charm disk storageAccountType using volume name.
		expected_charm="${charm_storage_acct_types[$i]:-}"
		charm_acct_type=$(az disk show -g "${rg}" -n "${volume}" --query "sku.name" -o tsv 2>/dev/null || true)
		if [[ ${charm_acct_type} != "${expected_charm}" ]]; then
			echo "MISMATCH: charm disk for ${app} expected ${expected_charm} but got ${charm_acct_type}" >>"${file}"
			return 1
		else
			echo "OK: charm disk for ${app} matches expected ${expected_charm}" >>"${file}"
		fi

	done
	echo "check_storage_account_type_controller run for ${controller_name} completed successfully." >>"${file}"
}

# Wait for graceful cloud cleanup before starting the next scenario.
cleanup_storage_account_type_controller() {
	local controller_name="${1}"
	local file="${TEST_DIR}/${controller_name}-destroy-controller.log"
	local result registry_tmp

	if [[ -n ${SKIP_DESTROY:-} ]]; then
		# Preserve the opt-out, including its overlapping resource usage.
		echo "====> Skipping destroy controller ${controller_name}"
		return 0
	fi

	echo "====> Destroying controller ${controller_name} before continuing"
	if timeout "${DESTROY_TIMEOUT}" juju destroy-controller --destroy-all-models --destroy-storage --no-prompt "${controller_name}" >"${file}" 2>&1; then
		cat "${file}" || return 1
	else
		result=$?
		echo "ERROR: destroying controller ${controller_name} failed (${result}); retaining cleanup tracking"
		cat "${file}" || return 1
		return "${result}"
	fi

	# Only remove this exact controller after synchronous destruction succeeds.
	registry_tmp=$(mktemp "${TEST_DIR}/jujus.XXXXXX") || return 1
	if grep -Fvx -- "${controller_name}" "${TEST_DIR}/jujus" >"${registry_tmp}"; then
		result=0
	else
		result=$?
	fi
	# grep returns 1 when the owned controller was the last entry, not on I/O failure.
	if [[ ${result} -gt 1 ]]; then
		echo "ERROR: could not read controller cleanup tracking"
		rm -f "${registry_tmp}" || return 1
		return "${result}"
	fi
	if ! mv "${registry_tmp}" "${TEST_DIR}/jujus"; then
		rm -f "${registry_tmp}" || return 1
		return 1
	fi
}

run_storage_account_type() {
	local default_controller_name="azure-default-controller"
	local premium_controller_name="azure-premium-controller"

	check_storage_account_type_controller "${default_controller_name}" "" "StandardSSD_LRS" || return 1
	cleanup_storage_account_type_controller "${default_controller_name}" || return 1
	check_storage_account_type_controller "${premium_controller_name}" "--storage-pool name=azp --storage-pool type=azure --storage-pool account-type=Premium_LRS --bootstrap-constraints=root-disk-source=azp" "Premium_LRS" || return 1
	cleanup_storage_account_type_controller "${premium_controller_name}" || return 1
}

test_storage_account_type() {
	if [ "$(skip 'test_storage_account_type')" ]; then
		echo "==> TEST SKIPPED: azure account-type"
		return
	fi

	if [ "$(az account list | yq e 'length' -)" -lt 1 ]; then
		echo "==> TEST SKIPPED: not logged in to Azure cloud"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_storage_account_type" "$@"
	)
}
