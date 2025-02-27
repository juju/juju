#!/bin/bash

# Test that model config default storage type is used when not specified.
# This test verifies that when storage type is not explicitly specified,
# Juju uses the default storage type from model configuration.
run_model_storage_iaas() {
	echo

	model_name="model-storage-iaas"
	file="${TEST_DIR}/test-${model_name}.log"

	ensure "${model_name}" "${file}"

	#test_default_block_storage "${model_name}"
	test_default_filesystem_storage "${model_name}"

	destroy_model "${model_name}"
}

# Test block storage uses the model's default storage type
test_default_block_storage() {
	local model_name=$1

	echo "Testing model config block storage type"

	# Set default block storage type in model config
	juju model-config -m "${model_name}" storage-default-block-source=loop

	# Deploy application with block storage without specifying storage type
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage-lp --storage disks=1G

	# Wait for the application to be active
	wait_for "dummy-storage-lp" "$(active_condition "dummy-storage-lp" 0)"
	wait_for "active" "$(workload_status "dummy-storage-lp" 0).current"

	# Clean up
	juju remove-application -m "${model_name}" --force --no-prompt dummy-storage
	wait_for "{}" ".applications"
}

# Test filesystem storage uses the model's default storage type
test_default_filesystem_storage() {
	local model_name=$1

	echo "Testing model config filesystem storage type with tmpfs"

	# Set default filesystem storage type in model config
	juju model-config -m "${model_name}" storage-default-filesystem-source=tmpfs

	# Deploy application with filesystem storage without specifying storage type
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage --storage single-fs=1G

	# Wait for the application to be active
	wait_for "dummy-storage" "$(active_condition "dummy-storage" 0)"
	wait_for "active" "$(workload_status "dummy-storage" 0).current"

	# Clean up
	juju remove-application -m "${model_name}" --force --no-prompt dummy-storage
	wait_for "{}" ".applications"

	echo "Testing model config filesystem storage type with rootfs"

	# Set default filesystem storage type in model config
	juju model-config -m "${model_name}" storage-default-filesystem-source=rootfs

	# Deploy application with filesystem storage without specifying storage type
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage --storage single-fs=1G

	# Wait for the application to be active
	wait_for "dummy-storage" "$(active_condition "dummy-storage" 0)"
	wait_for "active" "$(workload_status "dummy-storage" 1).current"

	# Clean up
	juju remove-application -m "${model_name}" --force --no-prompt dummy-storage
	wait_for "{}" ".applications"

	echo "All model config filesystem storage tests PASSED"
}

# Test model config default storage type for CAAS environments
run_model_storage_caas() {
	# Skip if not running with k8s provider
	if [ "${BOOTSTRAP_PROVIDER:-}" != "k8s" ]; then
		echo "Skipping CAAS storage test as provider is not k8s"
		return 0
	fi

	echo
	model_name="model-storage-caas"
	file="${TEST_DIR}/test-${model_name}.log"

	ensure "${model_name}" "${file}"

	# Set default storage type in model config for k8s
	juju model-config -m "${model_name}" storage-default-block-source=kubernetes

	# Deploy a k8s charm with storage
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage --storage database=1G

	# Wait for the application to be active
	wait_for "dummy-storage" "$(active_condition "dummy-storage" 0)"
	wait_for "active" "$(workload_status "dummy-storage" 0).current"

	# Clean up
	juju remove-application -m "${model_name}" --force --no-prompt dummy-storage
	wait_for "{}" ".applications"

	echo "All model config storage CAAS tests PASSED"

	destroy_model "${model_name}"
}

# Main test function that runs all the subtests
test_model_storage() {
	if [ "$(skip 'test_model_storage')" ]; then
		echo "==> TEST SKIPPED: model storage type tests"
		return
	fi

	(
		set_verbosity
		cd .. || exit

		run "run_model_storage_iaas"

		# Run CAAS test only if k8s provider
		if [ "${BOOTSTRAP_PROVIDER:-}" == "k8s" ]; then
			run "run_model_storage_caas"
		fi
	)
}
