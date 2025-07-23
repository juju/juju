test_add_unit_attach_storage() {
	if [ "$(skip 'test_add_unit_attach_storage')" ]; then
		echo "==> TEST SKIPPED: test_add_unit_attach_storage"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="add-unit-attach-storage"
	second_model_name="add-unit-attach-storage-second"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Create a PersistentVolume by deploying and deleting an application.
	juju deploy postgresql-k8s --channel 14/stable --trust -n 3
	# Ensure the storage is attached without waiting for the application to reach the active status.
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Capture the provisioned PersistentVolume ID.
	PV_0=$(juju storage --format json | jq -r '.volumes["0"]."provider-id"')
	PV_1=$(juju storage --format json | jq -r '.volumes["1"]."provider-id"')
	PV_2=$(juju storage --format json | jq -r '.volumes["2"]."provider-id"')

	# Clean up: remove the application and associated storage (retain PV).
	juju remove-application postgresql-k8s --no-prompt --force
	wait_for "{}" ".applications"
	juju remove-storage pgdata/0 --no-destroy
	juju remove-storage pgdata/1 --no-destroy
	juju remove-storage pgdata/2 --no-destroy
	wait_for "{}" ".storage"

	# Prepare PersistentVolumes for reuse: set reclaim policy to Retain and remove claimRef.
	for pv in "${PV_0}" "${PV_1}" "${PV_2}"; do
		kubectl patch pv "${pv}" -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
		PVC=$(kubectl get pv "${pv}" -o jsonpath='{.spec.claimRef.name}')
		kubectl delete pvc "${PVC}" -n "${model_name}" --ignore-not-found=true
		kubectl patch pv "${pv}" --type merge -p '{"spec":{"claimRef": null}}'
	done

	juju add-model "${second_model_name}"
	juju switch "${second_model_name}"

	for pv in "${PV_0}" "${PV_1}" "${PV_2}"; do
		juju import-filesystem kubernetes "${pv}" pgdata
	done

	# Deploy with --attach-storage. The storage should be attached to the psql-k8s/0 unit.
	juju deploy postgresql-k8s --channel 14/stable --trust --attach-storage pgdata/0 psql-k8s
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	juju add-unit psql-k8s --attach-storage pgdata/1
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	juju add-unit psql-k8s --attach-storage pgdata/2
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Verify PVs are bound and PVCs have correct labels
	for pv in "${PV_0}" "${PV_1}" "${PV_2}"; do
		OUT=$(kubectl get pv "${pv}" -o json | jq '.status.phase')
		echo "${OUT}" | check "Bound"

		NEW_PVC=$(kubectl get pv "${pv}" -o jsonpath='{.spec.claimRef.name}')
		PVC_JSON=$(kubectl get pvc -n "${second_model_name}" "${NEW_PVC}" -o json)

		echo "${PVC_JSON}" | jq '.metadata.labels."storage.juju.is/name"' | check "pgdata"
		echo "${PVC_JSON}" | jq '.metadata.labels."app.kubernetes.io/managed-by"' | check "juju"
		echo "${PVC_JSON}" | jq '.metadata.annotations."juju-storage-owner"' | check "psql-k8s"
	done

	# Verify volume provider IDs match the original PVs
	for i in 0 1 2; do
		eval "expected_pv=\$PV_${i}"
		OUT=$(juju storage --format json | jq ".volumes.\"${i}\".\"provider-id\"")
		# shellcheck disable=SC2154
		echo "${OUT}" | check "${expected_pv}"
	done

	# Destroy the test model.
	destroy_model "${model_name}"
	destroy_model "${second_model_name}"
}

test_add_unit_duplicate_pvc_exists() {
	if [ "$(skip 'test_add_unit_duplicate_pvc_exists')" ]; then
		echo "==> TEST SKIPPED: test_add_unit_duplicate_pvc_exists"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="add-unit-duplicate-pvc-exists"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Create a PersistentVolume by deploying and deleting an application.
	juju deploy postgresql-k8s --channel 14/stable --trust
	# Ensure the storage is attached without waiting for the application to reach the active status.
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Capture the provisioned PersistentVolume ID.
	PV=$(juju storage --format json | jq -r '.volumes["0"]."provider-id"')
	PVC=$(kubectl get pv "${PV}" -o jsonpath='{.spec.claimRef.name}')

	juju remove-unit postgresql-k8s --num-units 1 --force
	wait_for "null" '.applications."postgresql-k8s".units'

	# Patch PVC to have incorrect label to simulate duplicate PVC scenario
	kubectl patch pvc "${PVC}" \
		-n "${model_name}" \
		-p '{"metadata":{"labels":{"storage.juju.is/name":"not-pgdata"}}}'

	# Avoid race condition of attaching storage before kubectl patching completes
	attempt=0
	until kubectl get pvc "${PVC}" -n "${model_name}" -o json | jq -r '.metadata.labels."storage.juju.is/name"' | grep -q "not-pgdata"; do
		echo "[+] (attempt ${attempt}) waiting for PVC patch to complete"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))

		if [[ ${attempt} -gt 10 ]]; then
			echo "ERROR: failed waiting for PVC patch to complete"
			exit 1
		fi
	done

	# Attempt to add unit with --attach-storage (should fail due to incorrect PVC label)
	juju add-unit postgresql-k8s --attach-storage pgdata/0
	wait_for "error" '.applications."postgresql-k8s"."application-status".current'

	# Fix the PVC label to allow successful attachment
	kubectl patch pvc "${PVC}" \
		-n "${model_name}" \
		-p '{"metadata":{"labels":{"storage.juju.is/name":"pgdata"}}}'

	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Destroy the test model.
	destroy_model "${model_name}"
}

test_add_unit_attach_storage_scaling_race_condition() {
	if [ "$(skip 'test_add_unit_attach_storage_scaling_race_condition')" ]; then
		echo "==> TEST SKIPPED: test_add_unit_attach_storage_scaling_race_condition"
		return
	fi

	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure a bootstrap Juju model exists.
	model_name="add-unit-attach-storage-scaling-race-condition"
	second_model_name="add-unit-attach-storage-scaling-race-condition-second"
	file="${TEST_DIR}/test-${model_name}.log"
	ensure "${model_name}" "${file}"

	# Create a PersistentVolume by deploying and deleting an application.
	juju deploy postgresql-k8s --channel 14/stable --trust -n 3
	# Ensure the storage is attached without waiting for the application to reach the active status.
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'

	# Capture the provisioned PersistentVolume ID.
	PV_0=$(juju storage --format json | jq -r '.volumes["0"]."provider-id"')
	PV_1=$(juju storage --format json | jq -r '.volumes["1"]."provider-id"')
	PV_2=$(juju storage --format json | jq -r '.volumes["2"]."provider-id"')

	# Clean up: remove the application and associated storage (retain PV).
	juju remove-application postgresql-k8s --no-prompt --force
	wait_for "{}" ".applications"
	juju remove-storage pgdata/0 --no-destroy
	juju remove-storage pgdata/1 --no-destroy
	juju remove-storage pgdata/2 --no-destroy
	wait_for "{}" ".storage"

	# Prepare PersistentVolumes for reuse: set reclaim policy to Retain and remove claimRef.
	for pv in "${PV_0}" "${PV_1}" "${PV_2}"; do
		kubectl patch pv "${pv}" -p '{"spec":{"persistentVolumeReclaimPolicy":"Retain"}}'
		PVC=$(kubectl get pv "${pv}" -o jsonpath='{.spec.claimRef.name}')
		kubectl delete pvc "${PVC}" -n "${model_name}" --ignore-not-found=true
		kubectl patch pv "${pv}" --type merge -p '{"spec":{"claimRef": null}}'
	done

	juju add-model "${second_model_name}"
	juju switch "${second_model_name}"

	for pv in "${PV_0}" "${PV_1}" "${PV_2}"; do
		juju import-filesystem kubernetes "${pv}" pgdata
	done

	# Deploy with --attach-storage. The storage should be attached to the psql-k8s/0 unit.
	juju deploy postgresql-k8s --channel 14/stable --trust --attach-storage pgdata/0 psql-k8s
	wait_for_storage "attached" '.storage["pgdata/0"]["status"].current'

	# Add unit and remove them immediately to make sure it wouldn't break the juju.
	juju add-unit psql-k8s --attach-storage pgdata/1 && juju add-unit psql-k8s --attach-storage pgdata/2
	wait_for_storage "attached" '.storage["pgdata/1"]["status"].current'
	wait_for_storage "attached" '.storage["pgdata/2"]["status"].current'
	juju remove-unit psql-k8s --num-units 2 && juju remove-unit psql-k8s --num-units 1
	wait_for_storage "detached" '.storage["pgdata/0"]["status"].current'
	wait_for "null" '.applications."psql-k8s".units'

	# Destroy the test model.
	destroy_model "${model_name}"
	destroy_model "${second_model_name}"
}
