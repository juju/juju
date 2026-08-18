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
	juju deploy "$(pack_charm ../testcharms/charms/dummy-storage-k8s)" \
		--resource ubuntu-image=public.ecr.aws/ubuntu/ubuntu:22.04 dummy-k8s-storage
	# Ensure the storage is attached without waiting for the application to reach the active status.
	wait_for_storage "attached" '.storage["data/0"]["status"].current'

	# Capture the provisioned PersistentVolume ID.
	PV=$(juju storage --format json | yq -r '.filesystems["0"]."provider-id"')

	# Clean up: remove the application and associated storage (retain PV).
	juju remove-application dummy-k8s-storage --no-prompt
	wait_for "{}" ".applications"
	juju remove-storage data/0 --no-destroy
	wait_for "{}" ".storage"

	# Juju 4.0 storage detach already sets the PV reclaim policy to Retain
	# and deletes the PVC. The k8s PV controller clears claimRef asynchronously;
	# clear it explicitly to keep the test deterministic. Error-path assertions
	# for reclaim policy and claimRef are covered by unit tests in
	# internal/provider/kubernetes/storage_test.go.
	PVC=$(kubectl get pv "${PV}" -o jsonpath='{.spec.claimRef.name}')
	if [ -n "${PVC}" ]; then
		kubectl delete pvc "${PVC}" -n "${model_name}" --ignore-not-found=true
	fi
	kubectl patch pv "${PV}" --type merge -p '{"spec":{"claimRef": null}}'

	# Final attempt: import the PersistentVolume successfully.
	OUT=$(juju import-filesystem kubernetes "${PV}" data 2>&1)

	wait_for_storage "detached" '.storage["data/1"]["status"].current'
	wait_for_storage "${PV}" '.filesystems["1"]."provider-id"'

	# Destroy the test model.
	destroy_model "${model_name}"
}

test_force_import_filesystem() {
	if [ "$(skip 'test_force_import_filesystem')" ]; then
		echo "==> TEST SKIPPED: test_force_import_filesystem"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="force-import-filesystem"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Create a PersistentVolume by deploying and deleting an application.
	echo "Create persistent volume to be imported"
	juju deploy "$(pack_charm ../testcharms/charms/dummy-storage-k8s)" \
		--resource ubuntu-image=public.ecr.aws/ubuntu/ubuntu:22.04 dummy-k8s-storage
	# Ensure the storage is attached without waiting for the application to reach the active status.
	wait_for_storage "attached" '.storage["data/0"]["status"].current'

	# Capture the provisioned PersistentVolume ID.
	PV=$(juju storage --format json | yq -r '.filesystems["0"]."provider-id"')

	# Clean up: remove the application and associated storage (retain PV).
	juju remove-application dummy-k8s-storage --no-prompt
	wait_for "{}" ".applications"
	juju remove-storage data/0 --no-destroy
	wait_for "{}" ".storage"

	# Detach already sets reclaim policy to Retain; clear claimRef explicitly
	# for determinism. The label-mismatch force rejection is covered by unit tests.
	PVC=$(kubectl get pv "${PV}" -o jsonpath='{.spec.claimRef.name}')
	if [ -n "${PVC}" ]; then
		kubectl delete pvc "${PVC}" -n "${model_name}" --ignore-not-found=true
	fi
	kubectl patch pv "${PV}" --type merge -p '{"spec":{"claimRef": null}}'

	# Final attempt: import the PersistentVolume successfully.
	OUT=$(juju import-filesystem kubernetes "${PV}" data --force 2>&1)

	wait_for_storage "detached" '.storage["data/1"]["status"].current'

	# Ensure pv imported & status is available.
	PVC=$(kubectl get pv "${PV}" -o jsonpath='{.spec.claimRef.name}')
	echo "${PVC}" | check ""
	RECLAIM_POLICY=$(kubectl get pv "${PV}" -o jsonpath='{.spec.persistentVolumeReclaimPolicy}')
	echo "${RECLAIM_POLICY}" | check "Retain"

	# Destroy the test model.
	destroy_model "${model_name}"
}
