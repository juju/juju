#!/usr/bin/env bash

# Test that model config default storage type is used when not specified.
# This test verifies that when storage type is not explicitly specified,
# Juju uses the default storage block type from model configuration.
run_model_storage_block() {
	echo

	model_name="model-storage-block"
	file="${TEST_DIR}/test-${model_name}.log"

	ensure "${model_name}" "${file}"

	# Determine block storage type based on provider.
	block_storage_type=""
	case "${BOOTSTRAP_PROVIDER:-}" in
	"aws")
		block_storage_type="ebs"
		;;
	"lxd")
		# Skip block storage for LXD.
		block_storage_type=""
		;;
	*)
		# Default to loop for other providers.
		block_storage_type="loop"
		;;
	esac

	# Run block storage test for all providers except LXD.
	if [ -n "${block_storage_type}" ]; then
		test_default_block_storage "${model_name}" "${block_storage_type}"
	else
		echo "Skipping block storage test for ${BOOTSTRAP_PROVIDER:-} provider"
	fi

	destroy_model "${model_name}"
}

# Generic test for block storage using the provided storage type.
test_default_block_storage() {
	local model_name=$1
	local storage_type=$2

	echo "Testing model config block storage type with ${storage_type}"

	# Set default block storage type in model config.
	juju model-config -m "${model_name}" storage-default-block-source=${storage_type}

	# Deploy application with block storage without specifying storage type.
	juju deploy -m "${model_name}" ./testcharms/charms/dummy-storage --storage single-blk=100M

	# Wait for the application to be active.
	wait_for "dummy-storage" "$(active_condition "dummy-storage" 0)"
	wait_for "active" "$(workload_status "dummy-storage" 0).current"

	# Verify storage details.
	wait_for_storage "attached" '.storage["single-blk/0"]["status"].current'
	assert_storage "block" "$(kind_name "single-blk" 0)"
	assert_storage "alive" "$(life_status "single-blk" 0)"
	assert_storage "dummy-storage/0" "$(unit_attachment "single-blk" 0 0)"

	# Clean up.
	juju remove-application -m "${model_name}" --force --no-prompt dummy-storage
	wait_for "{}" ".applications"
}

test_model_storage_block() {
	if [ "$(skip 'test_model_storage_block')" ]; then
		echo "==> TEST SKIPPED: model storage block type tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_storage_block"
	)
}
