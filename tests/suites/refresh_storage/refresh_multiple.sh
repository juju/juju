# Verify storage is attached to a unit.
assert_storage_attached() {
    # e.g. storage-refresher/0
    UNIT="$1"
    # e.g. awesome-fs/0
    STORAGE_NAME="$2"

    if juju status --format json | jq -e \
        --arg unit "$UNIT" \
        --arg storage "$STORAGE_NAME" '
        .storage.storage
        | to_entries[]
        | select(.key == $storage)
        | select(.value.status.current == "attached")
        | select(.value.attachments.units[$unit])
        ' >/dev/null
    then
        return 0
    else
        return 1
    fi
}

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
    actual_size=$(juju status --format json | jq -r \
        --arg storage "$storage_name" \
        '.storage.filesystems[]
         | select(.storage == $storage)
         | .size')

    if [[ -z "$actual_size" || "$actual_size" == "null" ]]; then
        echo $(red "ERROR: Storage '$storage_name' not found")
        return 1
    fi

    local attached
    attached=$(juju status --format json | jq -r \
        --arg storage "$storage_name" \
        --arg unit "$unit_name" \
        '.storage.filesystems[]
         | select(.storage == $storage)
         | .attachments.units[$unit] != null')

    if [[ "$attached" != "true" ]]; then
        echo $(red "ERROR: Storage '$storage_name' is not attached to unit '$unit_name'")
        return 1
    fi

    if (( actual_size >= min_size )); then
        return 0
    else
        return 1
    fi
}

# Tests changes towards minimal count property.
# Reject the refresh if the minimal count is increased.
# Allow the refresh if the minimal count is decreased.
run_change_minimal_count() {
  echo

    model_name="test-change-minimal-count"
    file="${TEST_DIR}/${model_name}.log"

    ensure "${model_name}" "${file}"

    # Deploy revision 1 as the baseline.
    juju deploy "storage-multiple-refresher" --revision 1 --channel latest/edge
    wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher")"
    wait_for "attached" '.storage.storage["awesome-fs/0"].status.current'
    wait_for "attached" '.storage.storage["awesome-fs/1"].status.current'
    assert_storage_attached "storage-multiple-refresher/0" "awesome-fs/0"
    assert_storage_attached "storage-multiple-refresher/0" "awesome-fs/1"
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/0" 3072
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/1" 3072

    # The app requires a minimal of 2 awesome-fs storage instances.
    # Trying to detach would violate the requirement.
    OUT=$(juju detach-storage awesome-fs/1 2>&1 || true)
    echo "$OUT" | check 'removing storage from unit would violate charm storage "awesome-fs" requirements of having minimum 2 storage instances'

    # Refresh charm to revision 2 which increases the minimal storage count to 3.
    # Reject this refresh.
    OUT=$(juju refresh "storage-multiple-refresher" --revision 2 2>&1 || true)
    echo "$OUT" | check 'storage definition "awesome-fs" new minimum count 3 exceeds existing minimum count 2'

    # Revision remains at 1.
	  wait_for "storage-multiple-refresher" "$(charm_rev "storage-multiple-refresher" 1)"

    # Refresh charm to revision 3 which decreases the minimal storage count to 1.
    # Allow this refresh.
    juju refresh "storage-multiple-refresher" --revision 3

    wait_for "storage-multiple-refresher" "$(charm_rev "storage-multiple-refresher" 3)"

    juju add-unit storage-multiple-refresher
    wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher" 1)"
    wait_for "attached" '.storage.storage["awesome-fs/2"].status.current'
    assert_storage_attached "storage-multiple-refresher/1" "awesome-fs/2"
    assert_storage_attached "storage-multiple-refresher/1" "awesome-fs/3"
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/2" 3072
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/3" 3072

    # Let's try detaching the storage instances so we can have 1 storage instance attached.
    # Detach a storage instance from old unit (storage-multiple-refresher/0).
    OUT=$(juju detach-storage awesome-fs/1 2>&1 || true)
    echo "$OUT" | check "detaching awesome-fs/1"
    # Detach a storage instance from new unit (storage-multiple-refresher/1).
    OUT=$(juju detach-storage awesome-fs/3 2>&1 || true)
    echo "$OUT" | check "detaching awesome-fs/3"

    destroy_model "$model_name"
}

# Tests changes towards the maximal count property.
# Reject refresh if the maximal count is decreased.
# Allow the refresh if the maximal count is increased.
run_change_maximal_count() {
  echo

    model_name="test-change-maximal-count"
    file="${TEST_DIR}/${model_name}.log"

    ensure "${model_name}" "${file}"

    # Deploy revision 1 as the baseline.
    juju deploy "storage-multiple-refresher" --revision 1 --channel latest/edge
    wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher")"
    wait_for "attached" '.storage.storage["awesome-fs/0"].status.current'
    wait_for "attached" '.storage.storage["awesome-fs/1"].status.current'
    assert_storage_attached "storage-multiple-refresher/0" "awesome-fs/0"
    assert_storage_attached "storage-multiple-refresher/0" "awesome-fs/1"
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/0" 3072
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/1" 3072

    # Add more storage exceeding the max count.
    # This should fail.
    OUT=$(juju add-storage storage-multiple-refresher/0 awesome-fs="rootfs,3G,6" 2>&1 || true)
    echo "$OUT" | check "storage directive \"awesome-fs\" request count 8 exceeds the charm's maximum count of 5"

    # Refresh charm to revision 5 which decreases the maximal storage count to 3.
    # Reject this refresh.
    OUT=$(juju refresh "storage-multiple-refresher" --revision 5 2>&1 || true)
    echo "$OUT" | check 'storage definition "awesome-fs" new maximum count 3 is less than existing maximum count 5'

    # Revision remains at 1.
	  wait_for "storage-multiple-refresher" "$(charm_rev "storage-multiple-refresher" 1)"

    # Refresh charm to revision 4 which increases the maximal storage count to 8.
    # Allow this refresh.
    juju refresh "storage-multiple-refresher" --revision 4

    wait_for "storage-multiple-refresher" "$(charm_rev "storage-multiple-refresher" 4)"

    juju add-unit storage-multiple-refresher
    wait_for "storage-multiple-refresher" "$(active_idle_condition "storage-multiple-refresher" 1)"
    wait_for "attached" '.storage.storage["awesome-fs/2"].status.current'
    assert_storage_attached "storage-multiple-refresher/1" "awesome-fs/2"
    assert_storage_attached "storage-multiple-refresher/1" "awesome-fs/3"
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/2" 3072
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/3" 3072

    # Let's try adding more storage to max the count to 8.
    # Recall the old unit failed to perform this.the It should work now :)
    juju add-storage storage-multiple-refresher/0 awesome-fs="rootfs,3G,6"
    wait_for "attached" '.storage.storage["awesome-fs/4"].status.current'
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/4" 3072
    wait_for "attached" '.storage.storage["awesome-fs/5"].status.current'
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/5" 3072
    wait_for "attached" '.storage.storage["awesome-fs/6"].status.current'
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/6" 3072
    wait_for "attached" '.storage.storage["awesome-fs/7"].status.current'
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/7" 3072
    wait_for "attached" '.storage.storage["awesome-fs/8"].status.current'
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/8" 3072
    wait_for "attached" '.storage.storage["awesome-fs/9"].status.current'
    assert_storage_min_size "storage-multiple-refresher/0" "awesome-fs/9" 3072
    # This one fails because it exceeds the total of 8.
    OUT=$(juju add-storage storage-multiple-refresher/0 awesome-fs="rootfs,3G,2" 2>&1 || true)
    echo "$OUT" | check "storage directive \"awesome-fs\" request count 10 exceeds the charm's maximum count of 8"

    # It should also work for the new unit too.
    juju add-storage storage-multiple-refresher/1 awesome-fs="rootfs,3G,6"
    wait_for "attached" '.storage.storage["awesome-fs/10"].status.current'
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/10" 3072
    wait_for "attached" '.storage.storage["awesome-fs/11"].status.current'
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/11" 3072
    wait_for "attached" '.storage.storage["awesome-fs/12"].status.current'
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/12" 3072
    wait_for "attached" '.storage.storage["awesome-fs/13"].status.current'
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/13" 3072
    wait_for "attached" '.storage.storage["awesome-fs/14"].status.current'
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/14" 3072
    wait_for "attached" '.storage.storage["awesome-fs/15"].status.current'
    assert_storage_min_size "storage-multiple-refresher/1" "awesome-fs/15" 3072
    # This one fails because it exceeds the total of 8.
    OUT=$(juju add-storage storage-multiple-refresher/1 awesome-fs="rootfs,3G,2" 2>&1 || true)
    echo "$OUT" | check "storage directive \"awesome-fs\" request count 10 exceeds the charm's maximum count of 8"

    destroy_model "$model_name"
}

test_refresh_charm_storage_multiple() {
	if [ "$(skip 'test_refresh_charm_storage_multiple')" ]; then
		echo "==> TEST SKIPPED: refresh charm storage multiple"
		return
	fi

	(
		set_verbosity

		cd .. || exit

    # Tests changes in minimal count property.
    run "run_change_minimal_count"

    # Tests changes in maximal count property.
    run "run_change_maximal_count"
	)
}
