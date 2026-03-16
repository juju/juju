cleanup_deny_postgresql_sts_policy() {
	local main_sh_dir
	local deny_postgresql_sts_spec

	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	deny_postgresql_sts_spec="${main_sh_dir}/suites/storage_k8s/specs/deny-postgresql-sts.yaml"
	microk8s kubectl delete -f "${deny_postgresql_sts_spec}" --ignore-not-found=true >/dev/null 2>&1 || true
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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for 3 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Check that the containers in postgresql-k8s-0 pod should not restart
	microk8s kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq -o json '[.status.containerStatuses[].restartCount] | add' | check 0

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' |
		check 1024
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' |
		check 2048
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' |
		check 2048

	destroy_model "${model_name}"
}

# Scenario: issue storage and scale operations in quick succession in both orders.
# Expected outcome: app converges and new units are attached. The immediate new unit
# can validly have either old or new storage size (2GiB or 3GiB, then 3GiB or 4GiB)
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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Issue a storage update and scale quickly one after the other.
	juju application-storage postgresql-k8s pgdata=3G,kubernetes &&
		juju scale-application postgresql-k8s 2

	# Wait for app to stabilize.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for 2 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'

	# Check that the first unit uses 1GB storage and the second unit
	# can be 2GB or 3GB depending on reconcile ordering.
	# We cannot guarantee ordering especially when successive commands are issued
	# very close to each other. So we check two possible outcomes.
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' |
		check 1024
	juju storage --format json |
		yq -o json -r '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' |
		check "^(2048|3072)$"

	# Check that volume claim template is 3GB.
	# This is expected despite the new pod MAY be spawned with old 2GB storage.
	microk8s kubectl get sts postgresql-k8s -n "${model_name}" -o json |
		yq -o json -r '.spec.volumeClaimTemplates[] | select(.metadata.name | startswith("postgresql-k8s-pgdata")) | .spec.resources.requests.storage' |
		check 3Gi

	# Now if you scale the new unit will receive 3GB storage.
	juju scale-application postgresql-k8s 3
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for 3 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'
	juju storage --format json |
		yq -o json -r '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' |
		check 3072

	# Now do it the other way around.
	# Issue a scale and storage update quickly one after the other.
	juju scale-application postgresql-k8s 4 &&
		juju application-storage postgresql-k8s pgdata=4G,kubernetes

	# Wait for app to stabilize.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for 4 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for_storage "attached" '.storage["pgdata/3"]["status"].current'

	# The newly spawned unit can use either 3GB or 4GB depending on
	# reconcile ordering.
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' |
		check 1024
	juju storage --format json |
		yq -o json -r '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' |
		check "^(2048|3072)$"
	juju storage --format json |
		yq -o json -r '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' |
		check 3072
	juju storage --format json |
		yq -o json -r '.volumes | to_entries[] | select(.value.storage == "pgdata/3") | .value.size' |
		check "^(3072|4096)$"

	# Check that volume claim template is 4GB.
	# This is expected despite the new pod MAY be spawned with old 3GB storage.
	microk8s kubectl get sts postgresql-k8s -n "${model_name}" -o json |
		yq -o json -r '.spec.volumeClaimTemplates[] | select(.metadata.name | startswith("postgresql-k8s-pgdata")) | .spec.resources.requests.storage' |
		check 4Gi

	# Now if you scale, the new unit will receive 4GB storage.
	juju scale-application postgresql-k8s 5
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for 5 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for_storage "attached" '.storage["pgdata/4"]["status"].current'
	juju storage --format json |
		yq -o json -r '.volumes | to_entries[] | select(.value.storage == "pgdata/4") | .value.size' |
		check 4096

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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Apply the admission policy to deny creating a statefulset for postgresql-k8s.
	microk8s kubectl apply -f "${deny_postgresql_sts_spec}"

	# Update the storage to 2GB.
	# This will trigger a statefulset delete (which succeeds) and a statefulset reapply
	# (which fails due to the admission policy).
	juju application-storage postgresql-k8s pgdata=2G

	# Wait until it reaches an error state. At this point the statefulset is missing.
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"
	OUT=$(microk8s kubectl get sts "postgresql-k8s" -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

	# Delete the admission policy so the worker can reapply the statefulset.
	cleanup_deny_postgresql_sts_policy
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	microk8s kubectl get sts "postgresql-k8s" -n "${model_name}" -o json 2>&1 |
		yq -o json '.metadata.name' | check "postgresql-k8s"

	# After scale-application and add-unit, we will have a total
	# of 3 units.
	juju scale-application postgresql-k8s 3

	# Wait until the application is active and storage for the new units
	# are attached.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Check that the containers in postgresql-k8s-0 pod should not restart
	microk8s kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json |
		yq -o json '[.status.containerStatuses[].restartCount] | add' | check 0

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' |
		check 1024
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' |
		check 2048
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' |
		check 2048

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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for 3 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' |
		check 1024
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' |
		check 2048
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' |
		check 2048

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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Apply admission policy to deny creating postgresql-k8s sts.
	microk8s kubectl apply -f "${deny_postgresql_sts_spec}"

	# Issuing update storage will fail.
	juju application-storage postgresql-k8s pgdata=2G
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"

	# The sts is missing and juju fails to recreate it due to admission policy.
	OUT=$(microk8s kubectl get sts postgresql-k8s -n "${model_name}" 2>&1 || true)
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
	wait_for 5 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"

	# New storage are attached.
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/3"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/4"]["status"].current'

	# Check that the first unit uses 1GB storage and the new units
	# use 2GB storage.
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' | check 1024
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' | check 2048
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' | check 2048
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/3") | .value.size' | check 2048
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/4") | .value.size' | check 2048

	# Let's try scaling down now. Repeat the steps above.
	microk8s kubectl apply -f "${deny_postgresql_sts_spec}"
	juju application-storage postgresql-k8s pgdata=3G,kubernetes
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"
	OUT=$(microk8s kubectl get sts postgresql-k8s -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

	# Scale down.
	juju scale-application postgresql-k8s 4
	juju scale-application postgresql-k8s 2

	# Resume scale down to 2 units.
	cleanup_deny_postgresql_sts_policy
	wait_for 2 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"

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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

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
	wait_for 4 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/3"]["status"].current'

	# All units should use the old size (1GB).
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' | check 1024
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' | check 1024
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' | check 1024
	juju storage --format json |
		yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/3") | .value.size' | check 1024

	# Scale again to see new units with 2GB storage.
	juju scale-application postgresql-k8s 6
	wait_for 6 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"

	# New units should use the new size (2GB).
	wait_for_storage "attached" '.storage["pgdata/4"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/5"]["status"].current'
	juju storage --format json | yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/4") | .value.size' | check 2048
	juju storage --format json | yq -o json '.volumes | to_entries[] | select(.value.storage == "pgdata/5") | .value.size' | check 2048

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
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for 3 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Deny sts creation/reapply, then trigger storage update.
	microk8s kubectl apply -f "${deny_postgresql_sts_spec}"
	juju application-storage postgresql-k8s pgdata=3G,kubernetes
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"
	OUT=$(microk8s kubectl get sts "postgresql-k8s" -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

	# Remove app while storage update is stuck.
	juju remove-application postgresql-k8s --no-prompt
	wait_for "{}" ".applications"

	destroy_model "${model_name}"
}
