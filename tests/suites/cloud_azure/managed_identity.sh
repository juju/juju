check_managed_identity_controller() {
	local name identity_name
	name=${1}
	identity_name=${2}

	cloud="azure"
	if [[ -n ${BOOTSTRAP_REGION:-} ]]; then
		cloud_region="${cloud}/${BOOTSTRAP_REGION}"
	else
		cloud_region="${cloud}"
	fi

	juju bootstrap "${cloud_region}" "${name}" \
		--show-log \
		--constraints="instance-role=${identity_name}" 2>&1 | OUTPUT "${file}"
	echo "${name}" >>"${TEST_DIR}/jujus"

	cred_name=${AZURE_CREDENTIAL_NAME:-credentials}
	cred=$(juju show-credential --controller "${name}" azure "${cred_name}" 2>&1 || true)
	check_contains "$cred" "managed-identity-path"

	juju switch controller
	juju add-unit -m controller controller -n 2
	wait_for_controller_machines 3
	wait_for_ha 3

	juju add-model test
	juju deploy ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

}

run_auto_managed_identity() {
	echo

	name="azure-auto-managed-identity"
	file="${TEST_DIR}/test-auto-managed-identity.log"

	check_managed_identity_controller ${name} "auto"
}

run_custom_managed_identity() {
	echo

	# Create the managed identity to use with the controller.
	group="jtest-$(xxd -l 6 -c 32 -p </dev/random)"
	role="jrole-$(xxd -l 6 -c 32 -p </dev/random)"
	identity_name=jmid
	cred_name=${AZURE_CREDENTIAL_NAME:-credentials}
	subscription=$(juju show-credential --client azure "${cred_name}" | yq .client-credentials.azure."${cred_name}".content.subscription-id)
	az account set -s "${subscription}"

	add_clean_func "run_cleanup_azure"
	az group create --name "${group}" --location westus
	echo "${group}" >>"${TEST_DIR}/azure-groups"
	az identity create --resource-group "${group}" --name jmid
	mid=$(az identity show --resource-group "${group}" --name jmid --query principalId --output tsv)
	az role definition create --role-definition "{
      \"Name\": \"${role}\",
      \"Description\": \"Role definition for a Juju controller\",
      \"Actions\": [
                \"Microsoft.Compute/*\",
                \"Microsoft.KeyVault/*\",
                \"Microsoft.Network/*\",
                \"Microsoft.Resources/*\",
                \"Microsoft.Storage/*\",
                \"Microsoft.ManagedIdentity/userAssignedIdentities/*\"
      ],
      \"AssignableScopes\": [
            \"/subscriptions/${subscription}\"
      ]
  }"
	rname=$(az role definition list --name "${role}" | jq -r '.[0].name')
	if [[ -n ${rname} ]]; then
		echo "${rname}" >>"${TEST_DIR}/azure-roles"
	fi
	raid=$(az role assignment create --assignee-object-id "${mid}" --assignee-principal-type "ServicePrincipal" --role "${role}" --scope "/subscriptions/${subscription}" | jq -r .id)
	if [[ -n ${raid} ]]; then
		echo "${raid}" >>"${TEST_DIR}/azure-role-assignments"
	fi

	name="azure-custom-managed-identity"
	file="${TEST_DIR}/test-custom-managed-identity.log"

	check_managed_identity_controller ${name} "${group}/${identity_name}"
}

run_cleanup_azure() {
	set +e

	echo "==> Removing resource groups"
	if [[ -f "${TEST_DIR}/azure-groups" ]]; then
		while read -r group; do
			az group delete -y --resource-group "${group}" >>"${TEST_DIR}/azure_cleanup"
		done <"${TEST_DIR}/azure-groups"
	fi
	echo "==> Removed resource groups"

	echo "==> Removing role assignments"
	if [[ -f "${TEST_DIR}/azure-role-assignments" ]]; then
		while read -r id; do
			az role assignment delete --ids "${id}" >>"${TEST_DIR}/azure_cleanup"
		done <"${TEST_DIR}/azure-role-assignments"
	fi
	echo "==> Removed role assignments"

	echo "==> Removing roles"
	if [[ -f "${TEST_DIR}/azure-roles" ]]; then
		while read -r name; do
			az role definition delete --name "${name}" >>"${TEST_DIR}/azure_cleanup"
		done <"${TEST_DIR}/azure-roles"
	fi
	echo "==> Removed roles"
}

test_managed_identity() {
	if [ "$(skip 'test_managed_identity')" ]; then
		echo "==> TEST SKIPPED: managed identity"
		return
	fi

	if [ "$(az account list | jq length)" -lt 1 ]; then
		echo "==> TEST SKIPPED: not logged in to Azure cloud"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_auto_managed_identity" "$@"
		run "run_custom_managed_identity" "$@"
	)
}
