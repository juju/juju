cleanup_deny_postgresql_sts_policy() {
	local main_sh_dir
	local deny_postgresql_sts_spec

	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	deny_postgresql_sts_spec="${main_sh_dir}/suites/storage_k8s/specs/deny-postgresql-sts.yaml"
	kubectl delete -f "${deny_postgresql_sts_spec}" --ignore-not-found=true >/dev/null 2>&1 || true
}

add_wrench_application_storage() {
	juju ssh -m controller controller/0 \
		'mkdir -p /var/lib/juju/wrench && echo "ensure-storage-fail" > /var/lib/juju/wrench/application-storage'
}

cleanup_wrench_application_storage() {
	juju ssh -m controller controller/0 \
		'mkdir -p /var/lib/juju/wrench && : > /var/lib/juju/wrench/application-storage' >/dev/null 2>&1 || true
}

add_wrench_scale_application() {
	juju ssh -m controller controller/0 \
		'mkdir -p /var/lib/juju/wrench && echo "ensure-scale-fail" > /var/lib/juju/wrench/scale-application'
}

cleanup_wrench_scale_application() {
	juju ssh -m controller controller/0 \
		'mkdir -p /var/lib/juju/wrench && : > /var/lib/juju/wrench/scale-application' >/dev/null 2>&1 || true
}

cleanup_storage_class() {
	local main_sh_dir
	local cool_sc
	local awesome_sc

	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	cool_sc="${main_sh_dir}/suites/storage_k8s/specs/coolstorageclass-sc.yaml"
	awesome_sc="${main_sh_dir}/suites/storage_k8s/specs/awesomestorageclass-sc.yaml"
	kubectl delete -f "${cool_sc}" --ignore-not-found=true >/dev/null 2>&1 || true
	kubectl delete -f "${awesome_sc}" --ignore-not-found=true >/dev/null 2>&1 || true
	rm -f "${cool_sc}"
	rm -f "${awesome_sc}"
}

storage_id_for_pod() {
	local model_name pod_name storage_name claim_name_prefix_regex
	local pod_json pvc_name pvc_json pv_name storage_id

	model_name=${1}
	pod_name=${2}
	storage_name=${3}
	claim_name_prefix_regex="$storage_name-.*-$pod_name$"

	pod_json=$(kubectl get pod "${pod_name}" -n "${model_name}" -o json 2>/dev/null || true)
	if [[ -z ${pod_json} ]]; then
		return 1
	fi

	pvc_name=$(echo "${pod_json}" | CLAIM_NAME_PREFIX="$claim_name_prefix_regex" yq -o json -r '
		[
			.spec.volumes[]?
			| select(.persistentVolumeClaim != null)
			| .persistentVolumeClaim.claimName
		] | map(select(test(strenv(CLAIM_NAME_PREFIX)))) | .[0] // ""
	')
	if [[ -z ${pvc_name} ]]; then
		return 1
	fi

	pvc_json=$(kubectl get pvc "${pvc_name}" -n "${model_name}" -o json 2>/dev/null || true)
	if [[ -z ${pvc_json} ]]; then
		return 1
	fi

	pv_name=$(echo "${pvc_json}" | yq -o json -r '.spec.volumeName // ""')
	if [[ -z ${pv_name} ]]; then
		return 1
	fi

	storage_id=$(juju storage --format json 2>/dev/null | PV_NAME="${pv_name}" yq -o json -r '
		.volumes | to_entries[]
		| select(.value["provider-id"] == strenv(PV_NAME))
		| .value.storage
	' | head -n 1)
	if [[ -z ${storage_id} ]]; then
		return 1
	fi

	echo "${storage_id}"
}

wait_for_active_units() {
	local app_name expected_count app_index

	app_name=${1}
	expected_count=${2}
	app_index=${3:-0}

	wait_for "${app_name}" "$(active_condition "${app_name}" "$app_index")"
	wait_for "${expected_count}" ".applications[\"${app_name}\"].units | map(select(.[\"workload-status\"].current == \"active\")) | length"
}

# Scenario: update storage first, then scale out with both scale-application and add-unit.
# Expected outcome: app converges to active with 3 units; existing unit keeps old storage
# size and new units use updated storage size.
test_scale_and_update_storage() {
	if [ "$(skip 'test_scale_and_update_storage')" ]; then
		echo "==> TEST SKIPPED: test_scale_and_update_storage"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="scale-and-update-storage"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	juju deploy postgresql-k8s --channel 14/stable --trust

	# Wait until the application is active.
	wait_for_active_units "postgresql-k8s" 1
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id\"][\"status\"].current"

	# Update the storage to 2GB.
	juju application-storage postgresql-k8s pgdata=2G
	# Wait to stabilize.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"

	# After scale-application and add-unit, we will have a total
	# of 3 units.
	juju scale-application postgresql-k8s 2
	juju add-unit postgresql-k8s

	# Wait until the application is active and storage for the new units
	# are attached.
	wait_for_active_units "postgresql-k8s" 3
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id\"][\"status\"].current"
	postgresql_k8s_2_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id\"][\"status\"].current"

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id\") | .value.size" |
		check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id\") | .value.size" |
		check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id\") | .value.size" |
		check 2048

	destroy_model "${model_name}"
}

# Scenario: resize storage, scale up, scale down to 0, scale back up.
# Expected outcome: existing units retain their original storage sizes after scaling back up.
test_scale_down_and_back_up_retains_storage_sizes() {
	if [ "$(skip 'test_scale_down_and_back_up_retains_storage_sizes')" ]; then
		echo "==> TEST SKIPPED: test_scale_down_and_back_up_retains_storage_sizes"
		return
	fi

	echo

	model_name="scale-down-and-back-up-retains-storage"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	juju deploy postgresql-k8s --channel 14/stable --trust
	wait_for_active_units "postgresql-k8s" 1
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id\"][\"status\"].current"

	# Update storage to 2GB.
	juju application-storage postgresql-k8s pgdata=2G
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"

	# Scale up to 3 units.
	juju scale-application postgresql-k8s 3
	wait_for_active_units "postgresql-k8s" 3
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id\"][\"status\"].current"
	postgresql_k8s_2_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id\"][\"status\"].current"

	# Verify no pod restarts on unit 0.
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	# Verify unit 0 has 1GB, units 1 and 2 have 2GB.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id\") | .value.size" |
		check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id\") | .value.size" |
		check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id\") | .value.size" |
		check 2048

	# Scale down to 0.
	juju scale-application postgresql-k8s 0
	wait_for 0 '.applications["postgresql-k8s"]."units" // {} | keys | length'

	# Scale back up to 3.
	juju scale-application postgresql-k8s 3
	wait_for_active_units "postgresql-k8s" 3

	# Units are recreated but PVCs are reused. We verify storage IDs are the same.
	postgresql_k8s_0_storage_id_after=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id_after\"][\"status\"].current"
	postgresql_k8s_1_storage_id_after=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id_after\"][\"status\"].current"
	postgresql_k8s_2_storage_id_after=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id_after\"][\"status\"].current"

	# Assert storage IDs are unchanged after scale down and back up.
	echo "${postgresql_k8s_0_storage_id_after}" | check "${postgresql_k8s_0_storage_id}"
	echo "${postgresql_k8s_1_storage_id_after}" | check "${postgresql_k8s_1_storage_id}"
	echo "${postgresql_k8s_2_storage_id_after}" | check "${postgresql_k8s_2_storage_id}"

	# Verify storage sizes are preserved: unit 0 still 1GB, units 1 and 2 still 2GB.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id_after\") | .value.size" |
		check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id_after\") | .value.size" |
		check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id_after\") | .value.size" |
		check 2048

	destroy_model "${model_name}"
}

# Scenario: issue storage and scale operations in quick succession in both orders.
# Expected outcome: app converges and new units are attached. The immediate new unit
# can validly have either old or new storage size (1GiB or 3GiB, then 3GiB or 4GiB)
# because commands are issued so close together that we cannot guarantee which event
# ordering the controller receives first; that ordering determines that unit's size.
test_scale_and_update_storage_successive() {
	if [ "$(skip 'test_scale_and_update_storage_successive')" ]; then
		echo "==> TEST SKIPPED: test_scale_and_update_storage_successive"
		return
	fi

	echo
	model_name="scale-and-update-storage-successive"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	juju deploy postgresql-k8s --channel 14/stable --trust
	wait_for_active_units "postgresql-k8s" 1
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id\"][\"status\"].current"

	# Issue a storage update and scale quickly one after the other.
	juju application-storage postgresql-k8s pgdata=3G,kubernetes &&
		juju scale-application postgresql-k8s 2

	# Wait for app to stabilize.
	wait_for_active_units "postgresql-k8s" 2
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id\"][\"status\"].current"

	# Check that the first unit uses 1GB storage and the second unit
	# can be 1GB or 3GB depending on reconcile ordering.
	# We cannot guarantee ordering especially when successive commands are issued
	# very close to each other. So we check two possible outcomes.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id\") | .value.size" |
		check 1024
	juju storage --format json |
		yq -o json -r ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id\") | .value.size" |
		check "^(1024|3072)$"

	# Check that volume claim template is 3GB.
	# This is expected despite the new pod MAY be spawned with old 1GB storage.
	kubectl get sts postgresql-k8s -n "${model_name}" -o json |
		yq -o json -r '.spec.volumeClaimTemplates[] | select(.metadata.name | test("postgresql-k8s-pgdata-*.")) | .spec.resources.requests.storage' |
		check 3Gi

	# Now if you scale the new unit will receive 3GB storage.
	juju scale-application postgresql-k8s 3
	wait_for_active_units "postgresql-k8s" 3
	postgresql_k8s_2_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id\"][\"status\"].current"
	juju storage --format json |
		yq -o json -r ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id\") | .value.size" |
		check 3072

	# Now do it the other way around.
	# Issue a scale and storage update quickly one after the other.
	juju scale-application postgresql-k8s 4 &&
		juju application-storage postgresql-k8s pgdata=4G,kubernetes

	# Wait for app to stabilize.
	wait_for_active_units "postgresql-k8s" 4
	postgresql_k8s_3_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-3" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_3_storage_id\"][\"status\"].current"

	# The newly spawned unit can use either 3GB or 4GB depending on
	# reconcile ordering.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id\") | .value.size" |
		check 1024
	juju storage --format json |
		yq -o json -r ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id\") | .value.size" |
		check "^(1024|3072)$"
	juju storage --format json |
		yq -o json -r ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id\") | .value.size" |
		check 3072
	juju storage --format json |
		yq -o json -r ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_3_storage_id\") | .value.size" |
		check "^(3072|4096)$"

	# Check that volume claim template is 4GB.
	# This is expected despite the new pod MAY be spawned with old 3GB storage.
	kubectl get sts postgresql-k8s -n "${model_name}" -o json |
		yq -o json -r '.spec.volumeClaimTemplates[] | select(.metadata.name | test("postgresql-k8s-pgdata-*.")) | .spec.resources.requests.storage' |
		check 4Gi

	# Now if you scale, the new unit will receive 4GB storage.
	juju scale-application postgresql-k8s 5
	wait_for_active_units "postgresql-k8s" 5
	postgresql_k8s_4_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-4" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_4_storage_id\"][\"status\"].current"
	juju storage --format json |
		yq -o json -r ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_4_storage_id\") | .value.size" |
		check 4096

	# Check that the containers in existing pods 0-3 should not restart.
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0
	kubectl get pod postgresql-k8s-1 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0
	kubectl get pod postgresql-k8s-2 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0
	kubectl get pod postgresql-k8s-3 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	destroy_model "${model_name}"
}

# Scenario: storage update deletes sts, sts reapply is denied by policy, then policy is removed.
# Expected outcome: app self-heals back to active and subsequent scale-out succeeds with
# updated storage on new units.
test_scale_app_with_updated_storage_self_healing() {
	if [ "$(skip 'test_scale_app_with_updated_storage_self_healing')" ]; then
		echo "==> TEST SKIPPED: test_scale_app_with_updated_storage_self_healing"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="scale-app-with-updated-storage-self-healing"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"
	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	deny_postgresql_sts_spec="${main_sh_dir}/suites/storage_k8s/specs/deny-postgresql-sts.yaml"
	add_clean_func "cleanup_deny_postgresql_sts_policy"

	# Backstop cleanup in case a previous run crashed and left the policy around.
	cleanup_deny_postgresql_sts_policy

	# Wait until the application is active.
	juju deploy postgresql-k8s --channel 14/stable --trust
	wait_for_active_units "postgresql-k8s" 1
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id\"][\"status\"].current"

	# Apply the admission policy to deny creating a statefulset for postgresql-k8s.
	kubectl apply -f "${deny_postgresql_sts_spec}"

	# Update the storage to 2GB.
	# This will trigger a statefulset delete (which succeeds) and a statefulset reapply
	# (which fails due to the admission policy).
	juju application-storage postgresql-k8s pgdata=2G

	# Wait until it reaches an error state. At this point the statefulset is missing.
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"
	OUT=$(kubectl get sts "postgresql-k8s" -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

	# Delete the admission policy so the worker can reapply the statefulset.
	cleanup_deny_postgresql_sts_policy
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	kubectl get sts "postgresql-k8s" -n "${model_name}" -o json 2>&1 |
		yq -o json '.metadata.name' | check "postgresql-k8s"

	# After scale-application and add-unit, we will have a total
	# of 3 units.
	juju scale-application postgresql-k8s 3

	# Wait until the application is active and storage for the new units
	# are attached.
	wait_for_active_units "postgresql-k8s" 3
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id\"][\"status\"].current"
	postgresql_k8s_2_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id\"][\"status\"].current"

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id\") | .value.size" |
		check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id\") | .value.size" |
		check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id\") | .value.size" |
		check 2048

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	destroy_model "${model_name}"
}

# Scenario: update storage, crash before it's able to delete the statefulset,
# then issue a scale.
# Expected outcome: successfully reapplies statefulset with the new storage and
# the new pods spawn with the updated storage.
test_scale_after_storage_update_crash() {
	if [ "$(skip 'test_scale_after_storage_update_crash')" ]; then
		echo "==> TEST SKIPPED: test_scale_after_storage_update_crash"
		return
	fi

	echo
	model_name="scale-after-storage-update-crash-before-delete"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"
	add_clean_func "cleanup_wrench_application_storage"

	juju deploy postgresql-k8s --channel 14/stable --trust
	wait_for_active_units "postgresql-k8s" 1
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id\"][\"status\"].current"

	# Add a feature flag to purposely fail the storage update.
	add_wrench_application_storage

	# Updating the storage will cause the app status to error.
	juju application-storage postgresql-k8s pgdata=2G
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"

	# Issue a scale. The intent will be recorded but it won't start the last operation
	# that is yet to resume is a storage update.
	juju scale-application postgresql-k8s 3

	# While storage update is crashing, the app should not scale out yet.
	juju status postgresql-k8s --format json |
		yq -o json -r '.applications["postgresql-k8s"]."units" | keys | length' | check 1

	# Remove the feature flag so we can resume storage update.
	cleanup_wrench_application_storage

	# It will safe heal. Wait for the app to be active and new storage attached.
	wait_for_active_units "postgresql-k8s" 3
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id\"][\"status\"].current"
	postgresql_k8s_2_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id\"][\"status\"].current"

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id\") | .value.size" |
		check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id\") | .value.size" |
		check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id\") | .value.size" |
		check 2048

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	destroy_model "${model_name}"
}

# Scenario: update storage, delete statefulset, crash before it's able to reapply,
# then issue a scale.
# Expected outcome: successfully reapplies statefulset with new storage, the new
# pods spawn with updated storage.
test_scale_resumes_after_storage_update_missing_sts() {
	if [ "$(skip 'test_scale_resumes_after_storage_update_missing_sts')" ]; then
		echo "==> TEST SKIPPED: test_scale_resumes_after_storage_update_missing_sts"
		return
	fi

	echo
	model_name="scale-resumes-after-storage-update-missing-sts"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"
	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	deny_postgresql_sts_spec="${main_sh_dir}/suites/storage_k8s/specs/deny-postgresql-sts.yaml"
	add_clean_func "cleanup_deny_postgresql_sts_policy"

	cleanup_deny_postgresql_sts_policy

	juju deploy postgresql-k8s --channel 14/stable --trust
	wait_for_active_units "postgresql-k8s" 1
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id\"][\"status\"].current"

	# Apply admission policy to deny creating postgresql-k8s sts.
	kubectl apply -f "${deny_postgresql_sts_spec}"

	# Issuing update storage will fail.
	juju application-storage postgresql-k8s pgdata=2G
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"

	# The sts is missing and juju fails to recreate it due to admission policy.
	OUT=$(kubectl get sts postgresql-k8s -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

	# Let's try scaling multiple times while sts is missing. We record the intent
	# to scale but scaling cannot start because of failure to recreate sts.
	juju scale-application postgresql-k8s 3
	juju scale-application postgresql-k8s 5

	# The number of units should still be at 1.
	juju status postgresql-k8s --format json |
		yq -o json -r '.applications["postgresql-k8s"]."units" | keys | length' | check 1

	# Delete the admission policy and juju will safe heal.
	cleanup_deny_postgresql_sts_policy

	# We shoud have a total of 5 units.
	wait_for_active_units "postgresql-k8s" 5

	# New storage are attached.
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id\"][\"status\"].current"
	postgresql_k8s_2_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id\"][\"status\"].current"
	postgresql_k8s_3_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-3" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_3_storage_id\"][\"status\"].current"
	postgresql_k8s_4_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-4" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_4_storage_id\"][\"status\"].current"

	# Check that the first unit uses 1GB storage and the new units
	# use 2GB storage.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id\") | .value.size" | check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id\") | .value.size" | check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id\") | .value.size" | check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_3_storage_id\") | .value.size" | check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_4_storage_id\") | .value.size" | check 2048

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	# Let's try scaling down now. Repeat the steps above.
	kubectl apply -f "${deny_postgresql_sts_spec}"
	juju application-storage postgresql-k8s pgdata=3G,kubernetes
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"
	OUT=$(kubectl get sts postgresql-k8s -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

	# Scale down.
	juju scale-application postgresql-k8s 4
	juju scale-application postgresql-k8s 2

	# Resume scale down to 2 units.
	cleanup_deny_postgresql_sts_policy
	wait_for_active_units "postgresql-k8s" 2

	destroy_model "${model_name}"
}

# Scenario: issue a scale then crash, then issue storage update.
# Expected outcome: the new units spawn with the old storage. Statefulset is
# successfully reapplied with new storage.
test_storage_update_after_scale_crash() {
	if [ "$(skip 'test_storage_update_after_scale_crash')" ]; then
		echo "==> TEST SKIPPED: test_storage_update_after_scale_crash"
		return
	fi

	echo
	model_name="storage-update-after-scale-crash"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"
	add_clean_func "cleanup_wrench_scale_application"

	juju deploy postgresql-k8s --channel 14/stable --trust
	wait_for_active_units "postgresql-k8s" 1
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id\"][\"status\"].current"

	# Add a feature flag to fail the scale.
	add_wrench_scale_application

	# Scale the application. We record the intent but scaling fails to complete
	# due to feature flag. It should not add any units until the flag is removed.
	juju scale-application postgresql-k8s 4
	sleep 15
	wait_for 1 '.applications["postgresql-k8s"]."units" | keys | length'

	# Update the application. But this won't start until the scale operation completes.
	juju application-storage postgresql-k8s pgdata=2G

	# Remove the feature flag.
	cleanup_wrench_scale_application

	# Wait for app to stabilize.
	wait_for_active_units "postgresql-k8s" 4
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id\"][\"status\"].current"
	postgresql_k8s_2_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id\"][\"status\"].current"
	postgresql_k8s_3_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-3" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_3_storage_id\"][\"status\"].current"

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	# All units should use the old size (1GB).
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id\") | .value.size" | check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id\") | .value.size" | check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_2_storage_id\") | .value.size" | check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_3_storage_id\") | .value.size" | check 1024

	# Scale again to see new units with 2GB storage.
	juju scale-application postgresql-k8s 6
	wait_for_active_units "postgresql-k8s" 6

	# New units should use the new size (2GB).
	postgresql_k8s_4_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-4" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_4_storage_id\"][\"status\"].current"
	postgresql_k8s_5_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-5" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_5_storage_id\"][\"status\"].current"
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_4_storage_id\") | .value.size" | check 2048
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_5_storage_id\") | .value.size" | check 2048

	destroy_model "${model_name}"
}

# Scenario: remove an app while storage update is stuck because sts reapply is denied.
# Expected outcome: app can be removed cleanly while in error state.
test_remove_app_while_storage_update_stuck() {
	if [ "$(skip 'test_remove_app_while_storage_update_stuck')" ]; then
		echo "==> TEST SKIPPED: test_remove_app_while_storage_update_stuck"
		return
	fi

	echo
	model_name="remove-app-while-storage-update-stuck"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"
	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	deny_postgresql_sts_spec="${main_sh_dir}/suites/storage_k8s/specs/deny-postgresql-sts.yaml"
	add_clean_func "cleanup_deny_postgresql_sts_policy"

	# Backstop cleanup in case a previous run crashed and left the policy around.
	cleanup_deny_postgresql_sts_policy

	# Start with 3 replicas.
	juju deploy postgresql-k8s --channel 14/stable --trust -n 3
	wait_for_active_units "postgresql-k8s" 3
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_0_storage_id\"][\"status\"].current"
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_1_storage_id\"][\"status\"].current"
	postgresql_k8s_2_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-2" "pgdata")
	wait_for_storage "attached" ".storage[\"$postgresql_k8s_2_storage_id\"][\"status\"].current"

	# Deny sts creation/reapply, then trigger storage update.
	kubectl apply -f "${deny_postgresql_sts_spec}"
	juju application-storage postgresql-k8s pgdata=3G,kubernetes
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"
	OUT=$(kubectl get sts "postgresql-k8s" -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

	# Remove app while storage update is stuck.
	juju remove-application postgresql-k8s --no-prompt
	wait_for "{}" ".applications"

	destroy_model "${model_name}"
}

# Scenario: updating storage pool that uses different provider type.
# Expected outcome: update is rejected.
test_update_storage_constraints_validation_error() {
	if [ "$(skip 'test_update_storage_constraints_validation_error')" ]; then
		echo "==> TEST SKIPPED: test_update_storage_constraints_validation_error"
		return
	fi

	echo
	model_name="update-storage-constraints-validation-error"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	juju deploy postgresql-k8s --channel 14/stable db1 --storage=pgdata=kubernetes --trust
	wait_for_active_units "db1" 1 0
	OUT=$(juju application-storage db1 pgdata=tmpfs 2>&1 || true)
	echo "$OUT" | check 'cannot update storage constraints: updating current provider type: "kubernetes" to "tmpfs"'
	OUT=$(juju application-storage db1 pgdata=rootfs 2>&1 || true)
	echo "$OUT" | check 'cannot update storage constraints: updating current provider type: "kubernetes" to "rootfs"'

	juju deploy postgresql-k8s --channel 14/stable db2 --storage=pgdata=tmpfs --trust
	wait_for_active_units "db2" 1 1
	OUT=$(juju application-storage db2 pgdata=kubernetes 2>&1 || true)
	echo "$OUT" | check 'cannot update storage constraints: updating current provider type: "tmpfs" to "kubernetes"'
	OUT=$(juju application-storage db2 pgdata=rootfs 2>&1 || true)
	echo "$OUT" | check 'cannot update storage constraints: updating current provider type: "tmpfs" to "rootfs"'
	OUT=$(juju application-storage db2 pgdata=2GB 2>&1 || true)
	echo "$OUT" | check 'cannot update storage constraints: updating current size: 1024 to 2048'

	juju deploy postgresql-k8s --channel 14/stable db3 --storage=pgdata=rootfs --trust
	wait_for_active_units "db3" 1 2
	OUT=$(juju application-storage db3 pgdata=kubernetes 2>&1 || true)
	echo "$OUT" | check 'cannot update storage constraints: updating current provider type: "rootfs" to "kubernetes"'
	OUT=$(juju application-storage db3 pgdata=tmpfs 2>&1 || true)
	echo "$OUT" | check 'cannot update storage constraints: updating current provider type: "rootfs" to "tmpfs"'
	OUT=$(juju application-storage db3 pgdata=2GB 2>&1 || true)
	echo "$OUT" | check 'cannot update storage constraints: updating current size: 1024 to 2048'

	destroy_model "${model_name}"
}

# Scenario: updating storage pool that uses the same kubernetes provider.
# Expected outcome: update storage works because the pool is backed by the same
# provider type. After scaling down and up, the storage size and storage class
# is retained.
test_update_pool_same_provider_different_storage_class() {
	if [ "$(skip 'test_update_pool_same_provider_different_storage_class')" ]; then
		echo "==> TEST SKIPPED: test_update_pool_same_provider_different_storage_class"
		return
	fi

	echo
	model_name="update-pool-same-provider-different-storage-class"
	file="${TEST_DIR}/test-${model_name}.log"
	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	ensure "${model_name}" "${file}"

	# Get the default storage class
	current_storage_class=$(kubectl get sc -o json | yq -r '.items[0].metadata.name')

	# Clean up storage class in case there are leftovers from a previous run.
	cleanup_storage_class
	add_clean_func "cleanup_storage_class"

	# Dynamically get the name of the provisioner (different names in microk8s and
	# minikube but both contains hostpath in the name)
	provisioner=$(kubectl get sc -o yaml |
		yq -r '.items[] | select(.provisioner | test("hostpath")) | .provisioner' | head -n1)

	# Create a storage class called "coolstorageclass".
	cat <<EOF >"${main_sh_dir}/suites/storage_k8s/specs/coolstorageclass-sc.yaml"
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: coolstorageclass
  labels:
    addonmanager.kubernetes.io/mode: EnsureExists
provisioner: ${provisioner}
reclaimPolicy: Delete
volumeBindingMode: Immediate
EOF

	# Create a storage class called "awesomestorageclass".
	cat <<EOF >"${main_sh_dir}/suites/storage_k8s/specs/awesomestorageclass-sc.yaml"
  apiVersion: storage.k8s.io/v1
  kind: StorageClass
  metadata:
    name: awesomestorageclass
    labels:
      addonmanager.kubernetes.io/mode: EnsureExists
  provisioner: $provisioner
  reclaimPolicy: Delete
  volumeBindingMode: Immediate
EOF

	kubectl apply -f "${main_sh_dir}/suites/storage_k8s/specs/coolstorageclass-sc.yaml"
	kubectl apply -f "${main_sh_dir}/suites/storage_k8s/specs/awesomestorageclass-sc.yaml"

	# Unset the default class for current storage class.
	kubectl annotate sc "$current_storage_class" storageclass.kubernetes.io/is-default-class-

	# Create storage pool using our new storage class backed by kubernetes provider.
	juju create-storage-pool coolstoragepool kubernetes storage-class=coolstorageclass
	juju create-storage-pool awesomestoragepool kubernetes storage-class=awesomestorageclass

	# Deploy postgres, override pgdata storage to use coolstoragepool.
	juju deploy postgresql-k8s --channel 14/stable --storage=pgdata=coolstoragepool --trust --debug
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	postgresql_k8s_0_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")

	juju application-storage postgresql-k8s pgdata=awesomestoragepool,2GB
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"

	juju scale-application postgresql-k8s 2
	wait_for_active_units "postgresql-k8s" 2
	postgresql_k8s_1_storage_id=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq '.status.containerStatuses[].restartCount as $c ireduce (0; . + $c)' | check 0

	juju scale-application postgresql-k8s 0
	wait_for 0 '.applications["postgresql-k8s"]."units" // {} | keys | length'

	juju scale-application postgresql-k8s 2
	wait_for_active_units "postgresql-k8s" 2

	# Units are recreated but PVCs are reused. We verify storage IDs are the same.
	postgresql_k8s_0_storage_id_after=$(storage_id_for_pod "${model_name}" "postgresql-k8s-0" "pgdata")
	postgresql_k8s_1_storage_id_after=$(storage_id_for_pod "${model_name}" "postgresql-k8s-1" "pgdata")

	# Assert storage IDs are unchanged after scale down and back up.
	echo "${postgresql_k8s_0_storage_id_after}" | check "${postgresql_k8s_0_storage_id}"
	echo "${postgresql_k8s_1_storage_id_after}" | check "${postgresql_k8s_1_storage_id}"

	# Verify storage sizes are preserved: unit 0 still 1GB, units 1 still 2GB.
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_0_storage_id_after\") | .value.size" |
		check 1024
	juju storage --format json |
		yq -o json ".volumes | to_entries[] | select(.value.storage == \"$postgresql_k8s_1_storage_id_after\") | .value.size" |
		check 2048

	# Verify PV references the correct storage class names.
	provider_id_0=$(juju storage --format json |
		STORAGE_ID=$postgresql_k8s_0_storage_id_after yq -o json -r '.volumes[] | select(.storage == strenv(STORAGE_ID)) | ."provider-id"')
	provider_id_1=$(juju storage --format json |
		STORAGE_ID=$postgresql_k8s_1_storage_id_after yq -o json -r '.volumes[] | select(.storage == strenv(STORAGE_ID)) | ."provider-id"')

	kubectl get pv "$provider_id_0" -o json |
		yq -o json '.spec.storageClassName' |
		check "coolstorageclass"
	kubectl get pv "$provider_id_1" -o json |
		yq -o json '.spec.storageClassName' |
		check "awesomestorageclass"

	kubectl annotate sc "$current_storage_class" storageclass.kubernetes.io/is-default-class="true"
	destroy_model "${model_name}"
}
