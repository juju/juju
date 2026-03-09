cleanup_deny_postgresql_sts_policy() {
	local main_sh_dir
	local deny_postgresql_sts_spec

	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	deny_postgresql_sts_spec="${main_sh_dir}/suites/application_storage_k8s/specs/deny-postgresql-sts.yaml"
	kubectl delete -f "${deny_postgresql_sts_spec}" --ignore-not-found=true >/dev/null 2>&1 || true
}

cleanup_wrench_application_storage() {
	juju switch controller >/dev/null 2>&1 || true
	juju ssh controller/0 'mkdir -p /var/lib/juju/wrench && : > /var/lib/juju/wrench/application-storage' >/dev/null 2>&1 || true
}

cleanup_wrench_scale_application() {
	juju switch controller >/dev/null 2>&1 || true
	juju ssh controller/0 'mkdir -p /var/lib/juju/wrench && : > /var/lib/juju/wrench/scale-application' >/dev/null 2>&1 || true
}

test_scale_app_with_updated_storage() {
	if [ "$(skip 'test_scale_app_with_updated_storage')" ]; then
		echo "==> TEST SKIPPED: test_scale_app_with_updated_storage"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="scale-app-with-updated-storage"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	juju deploy postgresql-k8s --channel 14/stable --trust

	# Wait until the application is active.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Update the storage to 2GB.
	juju application-storage postgresql-k8s pgdata=2G,kubernetes

	# After scale-application and add-unit, we will have a total
	# of 3 units.
	juju scale-application postgresql-k8s 2
	juju add-unit postgresql-k8s

	# Wait until the application is active and storage for the new units
	# are attached.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json | jq '[.status.containerStatuses[].restartCount] | add' | check 0

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' | check 1024
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' | check 2048
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' | check 2048

	destroy_model "${model_name}"
}

# This tests self healing during a failure when reapplying a statefulset after it was deleted.
# We apply an admission policy to deny creating a statefulset for postgresql-k8s.
# This leaves us with a missing statefulset causing the app to go into an error status.
# The self healing will create the statefulset once the admission policy is removed.
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
	deny_postgresql_sts_spec="${main_sh_dir}/suites/application_storage_k8s/specs/deny-postgresql-sts.yaml"
	add_clean_func "cleanup_deny_postgresql_sts_policy"

	# Backstop cleanup in case a previous run crashed and left the policy around.
	kubectl delete -f "${deny_postgresql_sts_spec}" --ignore-not-found=true >/dev/null 2>&1 || true

  echo "[adis] deploying postgresql-k8s..."
	juju deploy postgresql-k8s --channel 14/stable --trust

	# Wait until the application is active.
	echo "[adis] waiting postgresql-k8s to become active..."
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Apply the admission policy to deny creating a statefulset for postgresql-k8s.
	echo "[adis] applying admission policy..."
	kubectl apply -f "${deny_postgresql_sts_spec}"

	# Update the storage to 2GB.
	# This will trigger a statefulset delete (which succeeds) and a statefulset reapply
	# (which fails due to the admission policy).
	echo "[adis] update storage to 2GB..."
	juju application-storage postgresql-k8s pgdata=2G,kubernetes

	# Wait until it reaches an error state. At this point the statefulset is missing.
	echo "[adis] waiting postgresql-k8s to become error..."
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"
	echo "[adis] fetching sts postgresql-k8s..."
	OUT=$(kubectl get sts "postgresql-k8s" -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

	# Delete the admission policy so the worker can reapply the statefulset.
	echo "[adis] delete admission policy..."
	kubectl delete -f "${deny_postgresql_sts_spec}"
	echo "[adis] wait postgresql-k8s to become active..."
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
  echo "[adis] fetching sts postgresql-k8s after going active again..."
	kubectl get sts "postgresql-k8s" -n "${model_name}" -o json 2>&1 | jq '.metadata.name' | check "postgresql-k8s"

	# After scale-application and add-unit, we will have a total
	# of 3 units.
	echo "[adis] scale postgresql-k8s..."
	juju scale-application postgresql-k8s 2
	echo "[adis] add unit postgresql-k8s..."
	juju add-unit postgresql-k8s

	# Wait until the application is active and storage for the new units
	# are attached.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Check that the containers in postgresql-k8s-0 pod should not restart
	kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json | jq '[.status.containerStatuses[].restartCount] | add' | check 0

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' | check 1024
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' | check 2048
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' | check 2048

	destroy_model "${model_name}"
}

# Scenario: update storage, crash before it's able to delete the statefulset,
# then issue a scale
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
	juju switch controller
	juju ssh controller/0 'mkdir -p /var/lib/juju/wrench && echo "ensure-storage-fail" > /var/lib/juju/wrench/application-storage'
	juju switch "${model_name}"

  # Updating the storage will cause the app status to error.
	juju application-storage postgresql-k8s pgdata=2G,kubernetes
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"

  # Issue a scale. The intent will be recorded but it won't start the last operation
  # that is yet to resume is a storage update.
	juju scale-application postgresql-k8s 3

	# While storage update is crashing, the app should not scale out yet.
	juju status postgresql-k8s --format json | jq -r '.applications["postgresql-k8s"]."units" | keys | length' | check 1

  # Remove the feature flag so we can resume storage update.
	juju switch controller
	juju ssh controller/0 'mkdir -p /var/lib/juju/wrench && : > /var/lib/juju/wrench/application-storage'
	juju switch "${model_name}"

  # It will safe heal. Wait for the app to be active and new storage attached.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
  wait_for 3 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

  # Check that the first unit uses 1GB storage and the new unit
  # uses 2GB storage.
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' | check 1024
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' | check 2048
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' | check 2048

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
	model_name="scale-after-storage-update-missing-sts-recovery"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"
	main_sh_dir="$(dirname "$(readlink -f "$0")")"
	deny_postgresql_sts_spec="${main_sh_dir}/suites/application_storage_k8s/specs/deny-postgresql-sts.yaml"
	add_clean_func "cleanup_deny_postgresql_sts_policy"

	kubectl delete -f "${deny_postgresql_sts_spec}" --ignore-not-found=true >/dev/null 2>&1 || true

	juju deploy postgresql-k8s --channel 14/stable --trust
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

  # Apply admission policy to deny creating postgresql-k8s sts.
	kubectl apply -f "${deny_postgresql_sts_spec}"

	# Issuing update storage will fail.
	juju application-storage postgresql-k8s pgdata=2G,kubernetes
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"

  # The sts is missing and juju fails to recreate it due to admission policy.
	OUT=$(kubectl get sts postgresql-k8s -n "${model_name}" 2>&1 || true)
	echo "$OUT" | check "NotFound"

  # Let's try scaling multiple times while sts is missing. We record the intent
  # to scale but scaling cannot start because of failure to recreate sts.
	juju scale-application postgresql-k8s 3
	juju scale-application postgresql-k8s 5

	# The number of units should still be at 1.
	juju status postgresql-k8s --format json | jq -r '.applications["postgresql-k8s"]."units" | keys | length' | check 1

  # Delete the admission policy and juju will safe heal.
	kubectl delete -f "${deny_postgresql_sts_spec}"

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
  juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' | check 1024
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' | check 2048
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' | check 2048
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/3") | .value.size' | check 2048
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/4") | .value.size' | check 2048

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
	kubectl delete -f "${deny_postgresql_sts_spec}"
	wait_for 2 '.applications["postgresql-k8s"]."units" | keys | length'
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"


	destroy_model "${model_name}"
}

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

	juju switch controller
	juju ssh controller/0 'mkdir -p /var/lib/juju/wrench && echo "ensure-scale-fail" > /var/lib/juju/wrench/scale-application'
	juju switch "${model_name}"

	juju scale-application postgresql-k8s 4
	juju application-storage postgresql-k8s pgdata=2G,kubernetes

	juju switch controller
	juju ssh controller/0 'mkdir -p /var/lib/juju/wrench && : > /var/lib/juju/wrench/scale-application'
	juju switch "${model_name}"

	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/3"]["status"].current'

	# First recovered scale should use the old size (1GiB).
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/3") | .value.size' | check 1024

	juju scale-application postgresql-k8s 6
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/5"]["status"].current'
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/5") | .value.size' | check 2048

	destroy_model "${model_name}"
}
