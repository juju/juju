test_import_filesystem() {
	if [ "$(skip 'test_import_filesystem')" ]; then
		echo "==> TEST SKIPPED: test_import_filesystem"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="import-filesystem"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Create a PersistentVolume by deploying and deleting an application.
	echo "Create persistent volume to be imported"
	juju deploy postgresql-k8s --channel 14/stable --trust
	# Ensure the storage is attached without waiting for the application to reach the active status.
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Capture the provisioned PersistentVolume ID.
	PV=$(juju storage --format json | jq -r '.volumes["0"]."provider-id"')

	# Clean up: remove the application and associated storage (retain PV).
	juju remove-application postgresql-k8s --no-prompt
	wait_for "{}" ".applications"
	juju remove-storage pgdata/0 --no-destroy
	wait_for "{}" ".storage"

	# Attempt to import the PersistentVolume: expect failure due to reclaim policy.
	set +e
	OUT=$(juju import-filesystem kubernetes "${PV}" pgdata 2>&1)
	set -e
	echo "${OUT}" | check \
		"importing volume \"${PV}\" with reclaim policy \"Delete\" not supported \(must be \"Retain\"\)"

	# Fix: update the PersistentVolume's reclaim policy to 'Retain'.
	kubectl patch pv "${PV}" -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'

	# Attempt to import the PersistentVolume: expect failure due to existing claimRef.
	set +e
	OUT=$(juju import-filesystem kubernetes "${PV}" pgdata 2>&1)
	set -e
	echo "${OUT}" | check \
		"importing volume \"${PV}\" already bound to a claim not supported"

	# Fix: delete the PVC and remove the claimRef from the PersistentVolume.
	PVC=$(kubectl get pv "${PV}" -o jsonpath='{.spec.claimRef.name}')
	kubectl delete pvc "${PVC}" -n "${model_name}"
	kubectl patch pv "${PV}" --type merge -p '{"spec":{"claimRef": null}}'

	# Final attempt: import the PersistentVolume successfully.
	OUT=$(juju import-filesystem kubernetes "${PV}" pgdata 2>&1)

	wait_for_storage "detached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "${PV}" '.volumes["1"]."provider-id"'

	# Destroy the test model.
	destroy_model "${model_name}"
}
