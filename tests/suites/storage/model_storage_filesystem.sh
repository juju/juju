#!/usr/bin/env bash

# Test that model config default storage type is used when not specified.
# This test verifies that when storage type is not explicitly specified,
# Juju uses the default storage filesystem type from model configuration.
run_model_storage_filesystem() {
	echo

	model_name="model-storage-filesystem"
	file="${TEST_DIR}/test-${model_name}.log"

	ensure "${model_name}" "${file}"

	# Run filesystem test for all providers.
	test_default_filesystem_storage "${model_name}"

	destroy_model "${model_name}"
}

# Test filesystem storage uses the model's default storage type.
test_default_filesystem_storage() {
	local model_name=$1

	# Get the default storage type from model config.
	default_fs=$(juju model-config -m "${model_name}" storage-default-filesystem-source)

	# Set the test storage type to the opposite of the default.
	test_fs="tmpfs"
	if [ "${default_fs}" = "tmpfs" ]; then
		test_fs="rootfs"
	fi

	echo "Testing model config filesystem storage type with ${test_fs}"

	# Set default filesystem storage type in model config.
	juju model-config -m "${model_name}" storage-default-filesystem-source=${test_fs}

	# Deploy application with filesystem storage without specifying storage type.
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage --storage single-fs=100M

	# Wait for the application to be active.
	wait_for "dummy-storage" "$(active_condition "dummy-storage" 0)"
	wait_for "active" "$(workload_status "dummy-storage" 0).current"

	# Verify tmpfs storage details.
	wait_for_storage "attached" '.storage["single-fs/0"]["status"].current'
	assert_storage "filesystem" "$(kind_name "single-fs" 0)"
	assert_storage "alive" "$(life_status "single-fs" 0)"
	assert_storage "dummy-storage/0" "$(unit_attachment "single-fs" 0 0)"

	# Clean up.
	juju remove-application -m "${model_name}" --force --no-prompt dummy-storage
	wait_for "{}" ".applications"
}

test_model_storage_filesystem() {
	if [ "$(skip 'test_model_storage_filesystem')" ]; then
		echo "==> TEST SKIPPED: model storage filesystem type tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_storage_filesystem"
	)
}
