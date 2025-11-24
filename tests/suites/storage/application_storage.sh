run_application_storage_directives() {
	echo

	model_name="application-storage"
	file="${TEST_DIR}/test-${model_name}.log"

	ensure "${model_name}" "${file}"
	app_name="postgresql"

	# Deploy an application with storage directive pgdata=2G.
	if [ "${BOOTSTRAP_PROVIDER:-}" = "k8s" ]; then
		echo "Deploying k8s application"
		app_name="postgresql-k8s"
		juju deploy "$app_name" --storage pgdata=2G --trust
	else
		echo "Deploying non-k8s application"
		app_name="postgresql"
		juju deploy "$app_name" --storage pgdata=2G
	fi

	# Wait for application to be active.
	wait_for true ".applications[\"$app_name\"][\"application-status\"].current==\"active\""

	juju application-storage "$app_name" --format=yaml
	old_size=$(juju application-storage "$app_name" --format=json | jq -r '.pgdata.Size')
	if [ "$old_size" != "2048" ]; then
		echo "Expected storage size 2048Mi, got $old_size"
		exit 1
	fi

	# For k8s, verify initial PVC size.
	if [ "${BOOTSTRAP_PROVIDER:-}" = "k8s" ]; then
		echo "Checking initial PVC size"

		# Get storageclass and PVC name (assumes single PVC for pgdata)
		pvc_name=$(kubectl -n "${model_name}" get pvc -o json | jq -r '.items[0].metadata.name')
		pvc_size=$(kubectl -n "${model_name}" get pvc "$pvc_name" -o json | jq -r '.spec.resources.requests.storage')

		if [ "$pvc_size" != "2Gi" ]; then
			echo "Expected PVC size 2Gi, got $pvc_size"
			exit 1
		fi
	fi

	# Update the storage directive pgdata=3G.
	juju application-storage "$app_name" pgdata=3G
	new_size=$(juju application-storage "$app_name" --format=json | jq -r '.pgdata.Size')
	if [ "$new_size" != "3072" ]; then
		echo "Expected updated storage size to be 3072Mi, got $new_size"
		exit 1
	fi

	# For k8s, verify the updated PVC size.
	if [ "${BOOTSTRAP_PROVIDER:-}" = "k8s" ]; then
		echo "Checking updated PVC size"

		pvc_name=$(kubectl -n "${model_name}" get pvc -o json | jq -r '.items[0].metadata.name')
		pvc_size=$(kubectl -n "${model_name}" get pvc "$pvc_name" -o json | jq -r '.spec.resources.requests.storage')

		if [ "$pvc_size" != "3Gi" ]; then
			echo "Expected updated PVC size 3Gi, got $pvc_size"
			exit 1
		fi
	fi

	# Add 2 more units and check their storage size is 3G.
	juju add-unit "$app_name" -n 2
	wait_for "${app_name}/1" ".applications"
	wait_for "${app_name}/2" ".applications"

	unit_1_size=$(juju storage --format=json | jq -r --arg u "${app_name}/1" \
		'.filesystems[] | select(.attachments.units[$u]) | .size')
	unit_2_size=$(juju storage --format=json | jq -r --arg u "${app_name}/2" \
		'.filesystems[] | select(.attachments.units[$u]) | .size')

	if [ "$unit_1_size" != "3072" ] || [ "$unit_2_size" != "3072" ]; then
		echo "Expected new units storage size to be 3072Mi, got $unit_1_size and $unit_2_size"
		exit 1
	fi

	destroy_model "${model_name}"
}

test_application_storage() {
	if [ "$(skip 'test_application_storage')" ]; then
		echo "==> TEST SKIPPED: application storage tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_application_storage_directives"
	)
}
