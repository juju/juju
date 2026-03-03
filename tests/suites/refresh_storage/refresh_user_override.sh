# Verify storage is attached and meets minimum size requirement.
assert_storage_min_size() {
	# e.g. storage-refresher/0
	local unit_name="$1"
	# e.g. awesome-fs/0
	local storage_name="$2"
	# 1024 (in MiB)
	local min_size="$3"

	if [[ -z "$unit_name" || -z "$storage_name" || -z "$min_size" ]]; then
		echo $(red "usage: assert_storage_min_size <unit> <storage> <min_size_mib>")
		return 1
	fi

	local actual_size
	actual_size=$(juju storage --format json | jq -r \
		--arg storage "$storage_name" \
		'.filesystems[]
         | select(.storage == $storage)
         | .size')

	if [[ -z "$actual_size" || "$actual_size" == "null" ]]; then
		echo $(red "ERROR: Storage '$storage_name' not found")
		return 1
	fi

	local attached
	attached=$(juju storage --format json | jq -r \
		--arg storage "$storage_name" \
		--arg unit "$unit_name" \
		'.filesystems[]
         | select(.storage == $storage)
         | .attachments.units[$unit] != null')

	if [[ "$attached" != "true" ]]; then
		echo $(red "ERROR: Storage '$storage_name' is not attached to unit '$unit_name'")
		return 1
	fi

	if ((actual_size >= min_size)); then
		return 0
	else
		return 1
	fi
}

# Tests the behavior of the system when invalid storage directives
# or unsupported storage pools are supplied during a charm refresh.
run_invalid_pool_and_storage_directive() {
	echo

	model_name="test-invalid-pool-and-storage-directive"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy revision 1 as the baseline.
	juju deploy "storage-multiple-refresher" --revision 1 --channel latest/edge
	wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher")"
	wait_for "attached" '.storage.storage["awesome-fs/0"].status.current'
	wait_for "attached" '.storage.storage["awesome-fs/1"].status.current'

	# Supply a pool that doesn't exist.
	OUT=$(juju refresh storage-multiple-refresher --revision 3 --storage awesome-fs="unknownpool,3G,1" 2>&1 || true)
	echo "$OUT" | check 'storage directive "awesome-fs" references unknown storage pool "unknownpool"'

	# Supply a storage directive that doesn't exist.
	OUT=$(juju refresh storage-multiple-refresher --revision 3 --storage unknown-fs="rootfs,3G,1" 2>&1 || true)
	echo "$OUT" | check 'storage directive "unknown-fs" does not exist in the charm'

	# Deploy revision 6 (storage type block) as the baseline.
	juju deploy "storage-multiple-refresher" --revision 6 --channel latest/edge another-storage-multiple-refresher
	wait_for "another-storage-multiple-refresher" "$(active_idle_condition "another-storage-multiple-refresher")"

	# Supply a pool that doesn't support the requested storage type. "rootfs" is a
	# pool with filesystem type, however, the charm requests block type.
	OUT=$(juju refresh another-storage-multiple-refresher --revision 7 --storage awesome-block="rootfs,3G" 2>&1 || true)
	echo "$OUT" | check 'storage directive pool .* does not support charm storage "block"'

	destroy_model "$model_name"
}

# Tests behavior of the system when overriding the storage count
# with values below the minimum, above the maximum, or exceeding uint32 limits.
run_override_count() {
	echo

	model_name="test-override-count"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy revision 1 as the baseline.
	juju deploy "storage-multiple-refresher" --revision 1 --channel latest/edge
	wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher")"
	wait_for "attached" '.storage.storage["awesome-fs/0"].status.current'
	wait_for "attached" '.storage.storage["awesome-fs/1"].status.current'

	# Supply a count below the minimum.
	OUT=$(juju refresh storage-multiple-refresher --revision 4 --storage awesome-fs="rootfs,3G,1" 2>&1 || true)
	echo "$OUT" | grep 'storage "awesome-fs" cannot have less than 2 storage instances'

	# Supply a count above the maximum.
	OUT=$(juju refresh storage-multiple-refresher --revision 4 --storage awesome-fs="rootfs,3G,10" 2>&1 || true)
	echo "$OUT" | grep 'storage "awesome-fs" cannot exceed 8 storage instances'

	# Supply a count exceeding uint32 limit.
	OUT=$(juju refresh storage-multiple-refresher --revision 4 --storage awesome-fs="rootfs,3G,9999999999" 2>&1 || true)
	echo "$OUT" | grep 'storage directive "awesome-fs" override count 9999999999 exceeds maximum 4294967295'

	# Supply a count within the limit.
	juju refresh storage-multiple-refresher --revision 4 --storage awesome-fs="rootfs,3G,3"
	wait_for "storage-multiple-refresher" "$(charm_rev "storage-multiple-refresher" 4)"

	juju add-unit storage-multiple-refresher
	wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher" 1)"
	wait_for "attached" '.storage.storage["awesome-fs/2"].status.current'
	wait_for "attached" '.storage.storage["awesome-fs/3"].status.current'
	wait_for "attached" '.storage.storage["awesome-fs/4"].status.current'

	destroy_model "$model_name"
}

# Tests the behavior of the system when overriding storage size
# during a charm refresh. Validates that:
#   - A size below the charm's minimum requirement is rejected.
#   - A size above the charm's minimum requirement is accepted and
#     applied to new units.
#   - A size equal to the charm's minimum requirement is accepted and
#     applied to new units.
run_override_size() {
	echo

	model_name="test-override-size"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy revision 1 as the baseline.
	juju deploy "storage-refresher" --revision 1 --channel latest/edge
	wait_for "storage-refresher" "$(active_idle_condition "storage-refresher")"
	wait_for "attached" '.storage.storage["awesome-fs/0"].status.current'

	# Refresh charm with invalid size override below minimum
	OUT=$(juju refresh storage-refresher --revision 3 --storage awesome-fs="rootfs,500M" 2>&1 || true)
	echo "$OUT" | check 'storage directive size 500 is less than the charm minimum requirement of 1024'

	# Refresh charm with valid size override above minimum
	juju refresh storage-refresher --revision 3 --storage awesome-fs="rootfs,5G"
	wait_for "storage-refresher" "$(charm_rev "storage-refresher" 3)"

	juju add-unit storage-refresher
	wait_for "storage-refresher" "$(active_idle_condition "storage-refresher" 1)"
	wait_for "attached" '.storage.storage["awesome-fs/1"].status.current'

	if ! assert_storage_min_size "storage-refresher/1" "awesome-fs/1" 5120; then
		echo $(red "attached storage is not at least 5120 in size")
		exit 1
	fi

	# Deploy another revision 1 as the baseline.
	juju deploy "storage-refresher" --revision 1 --channel latest/edge another-storage-refresher
	wait_for "another-storage-refresher" "$(active_idle_condition "another-storage-refresher")"
	wait_for "attached" '.storage.storage["awesome-fs/2"].status.current'

	# Refresh charm with valid size override equal to minimum.
	juju refresh another-storage-refresher --revision 3 --storage awesome-fs="rootfs,1G"
	wait_for "another-storage-refresher" "$(charm_rev "another-storage-refresher" 3)"

	juju add-unit another-storage-refresher
	wait_for "another-storage-refresher" "$(active_idle_condition "another-storage-refresher" 1)"
	wait_for "attached" '.storage.storage["awesome-fs/3"].status.current'

	if ! assert_storage_min_size "another-storage-refresher/1" "awesome-fs/3" 1024; then
		echo $(red "attached storage is not at least 1024 in size")
		exit 1
	fi

	destroy_model "$model_name"
}

# Tests the behavior of the system when overriding multiple storage
# properties (pool, size, and count) simultaneously during a charm refresh.
# Validates that:
#   - A valid size with an invalid count (exceeding max) is rejected.
#   - A valid count with an invalid size (below minimum) is rejected.
#   - A combination of valid pool, size, and count is accepted, and the
#     overridden properties are applied to new units.

run_override_mixed_properties() {
	echo

	model_name="test-override-mixed-properties"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# Deploy revision 1 as the baseline.
	juju deploy "storage-multiple-refresher" --revision 1 --channel latest/edge
	wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher")"
	wait_for "attached" '.storage.storage["awesome-fs/0"].status.current'
	wait_for "attached" '.storage.storage["awesome-fs/1"].status.current'

	# Despite the supplied min size respecting the charm's requirement, the refresh
	# fails because the supplied count exceeds the charm's max count of 8.
	OUT=$(juju refresh storage-multiple-refresher --revision 8 --storage awesome-fs="rootfs,2G,10" 2>&1 || true)
	echo "$OUT" | check 'storage "awesome-fs" cannot exceed 8 storage instances'

	# Despite the count respecting the charm's requirement, the refresh
	# fails because the supplied min size falls below the charm's min size of 1G.
	OUT=$(juju refresh storage-multiple-refresher --revision 8 --storage awesome-fs="rootfs,500M,5" 2>&1 || true)
	echo "$OUT" | check 'storage directive size 500 is less than the charm minimum requirement of 1024'

	# Refresh with mixed properties that respect the charm's requirement.
	juju refresh storage-multiple-refresher --revision 8 --storage awesome-fs="rootfs,7G,5"
	wait_for "storage-multiple-refresher" "$(charm_rev "storage-multiple-refresher" 8)"

	juju add-unit storage-multiple-refresher
	wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher" 1)"

	for i in $(seq 2 6); do
		wait_for "attached" ".storage.storage[\"awesome-fs/$i\"].status.current"
	done

	for i in $(seq 2 6); do
		if ! assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/$i" 7168; then
			echo $(red "attached storage awesome-fs/$i is not at least 7168 in size")
			exit 1
		fi
	done

	destroy_model "$model_name"
}

test_refresh_charm_storage_user_override() {
	if [ "$(skip 'test_refresh_charm_storage_user_override')" ]; then
		echo "==> TEST SKIPPED: refresh charm storage user override"
		return
	fi

	(
		set_verbosity

		cd .. || exit

    # Tests when the supplied pool and storage name is invalid.
		run "run_invalid_pool_and_storage_directive"

		# Tests supplying the count property.
		run "run_override_count"

		# Tests supplying the size property.
		run "run_override_size"

		# Tests supplying a mixture of valid and invalid properties.
		run "run_override_mixed_properties"
	)
}
