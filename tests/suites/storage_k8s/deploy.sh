test_deploy_attach_storage() {
	if [ "$(skip 'test_deploy_attach_storage')" ]; then
		echo "==> TEST SKIPPED: test_deploy_attach_storage"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="deploy-attach-storage"
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

	# Clean up: make sure PersistentVolume is in available status
	kubectl patch pv "${PV}" -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
	PVC=$(kubectl get pv "${PV}" -o jsonpath='{.spec.claimRef.name}')
	kubectl delete pvc "${PVC}" -n "${model_name}"
	kubectl patch pv "${PV}" --type merge -p '{"spec":{"claimRef": null}}'

	# Import filesystem as pgdata/1
	juju import-filesystem kubernetes "${PV}" pgdata
	wait_for_storage "detached" '.storage["pgdata/1"]["status"].current'

	# Deploy with --attach-storage. The storage should be attach to the psql-k8s/0.
	juju deploy postgresql-k8s --channel 14/stable --trust --attach-storage pgdata/1 psql-k8s
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'

	OUT=$(kubectl get pv "${PV}" -o json | jq -r '.status.phase')
	echo "${OUT}" | check "Bound"

	# Make sure new PV/PVC is used by the postgresql-k8s charm
	NEW_PVC=$(kubectl get pv "${PV}" -o jsonpath='{.spec.claimRef.name}')
	OUT=$(
		kubectl get pvc -n "${model_name}" "${NEW_PVC}" -o json |
			jq '.metadata.labels."storage.juju.is/name"'
	)
	echo "${OUT}" | check "pgdata"
	OUT=$(
		kubectl get pvc -n "${model_name}" "${NEW_PVC}" -o json |
			jq '.metadata.labels."app.kubernetes.io/name"'
	)
	echo "${OUT}" | check "psql-k8s"

	# Destroy the test model.
	destroy_model "${model_name}"
}
