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
	microk8s kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json | jq '[.status.containerStatuses[].restartCount] | add' | check 0

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
	if [ "$(skip 'scale_app_with_updated_storage_self_healing')" ]; then
		echo "==> TEST SKIPPED: scale_app_with_updated_storage_self_healing"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="scale-app-with-updated-storage-self-healing"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	juju deploy postgresql-k8s --channel 14/stable --trust

	# Wait until the application is active.
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Apply the admission policy to deny creating a statefulset for postgresql-k8s.
	microk8s kubectl apply -f "./tests/suites/application_storage/specs/deny-postgresql-sts.yaml"

	# Update the storage to 2GB.
	# This will trigger a statefulset delete (which succeeds) and a statefulset reapply
	# (which fails due to the admission policy).
	juju application-storage postgresql-k8s pgdata=2G,kubernetes

	# Wait until it reaches an error state. At this point the statefulset is missing.
	wait_for "postgresql-k8s" "$(error_condition "postgresql-k8s" 0)"
	microk8s kubectl get sts "postgresql-k8s" -n "${model_name}" -o json 2>&1 | check "NotFound"

	# Delete the admission policy so the worker can reapply the statefulset.
	microk8s kubectl delete -f "./tests/suites/application_storage/specs/deny-postgresql-sts.yaml"
	wait_for "postgresql-k8s" "$(active_condition "postgresql-k8s" 0)"
	microk8s kubectl get sts "postgresql-k8s" -n "${model_name}" -o json 2>&1 | jq '.metadata.name' | check "postgresql-k8s"

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
	microk8s kubectl get pod postgresql-k8s-0 -n "${model_name}" -o json | jq '[.status.containerStatuses[].restartCount] | add' | check 0

	# Check that the first unit uses 1GB storage and the new unit
	# uses 2GB storage.
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/0") | .value.size' | check 1024
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/1") | .value.size' | check 2048
	juju storage --format json | jq '.volumes | to_entries[] | select(.value.storage == "pgdata/2") | .value.size' | check 2048

	destroy_model "${model_name}"
}
