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

# Verifies that a given storage instance is attached to a specific unit and
# mounted at the expected location.
assert_storage_mount_location() {
    local unit_name="$1"
    local storage_name="$2"
    local expected_location="$3"

    if [[ -z "$unit_name" || -z "$storage_name" || -z "$expected_location" ]]; then
        echo $(red "usage: assert_storage_mount_location <unit> <storage> <expected_location>")
        return 1
    fi

    local actual_location
    actual_location=$(juju status --format json | jq -r \
        --arg storage "$storage_name" \
        --arg unit "$unit_name" \
        '.storage.filesystems[]
         | select(.storage == $storage)
         | .attachments.units[$unit].location')

    if [[ -z "$actual_location" || "$actual_location" == "null" ]]; then
        echo $(red "ERROR: Storage '$storage_name' not attached to unit '$unit_name'")
        return 1
    fi

    if [[ "$actual_location" == "$expected_location" ]]; then
        return 0
    else
        echo $(red "ERROR: Storage '$storage_name' mounted at '$actual_location', expected '$expected_location'")
        return 1
    fi
}

# Tests that when a charm is refreshed to a revision with identical storage definitions,
# existing and new units maintain the same storage size and mount locations.
run_no_changes_in_new_revision() {
  	echo

    	model_name="test-no-changes-in-new-revision"
    	file="${TEST_DIR}/${model_name}.log"

      ensure "${model_name}" "${file}"

      juju deploy "storage-refresher" --revision 1 --channel latest/edge
      wait_for "storage-refresher" "$(active_idle_condition "storage-refresher")"

      # Assert the new unit has the at least 3072 MiB.
      if ! assert_storage_min_size "storage-refresher/0" "awesome-fs/0" 3072 ; then
        echo $(red "attached storage is not at least 3072 in size")
        exit 1
      fi

      # Assert the new unit has the same mount location.
      if ! assert_storage_mount_location "storage-refresher/0" "awesome-fs/0" "/awesome-fs" ; then
        echo $(red "awesome-fs/1 is not located in /awesome-fs")
        exit 1
      fi

      # Refresh charm to revision 9 which has the exact contents as revision 1.
      juju refresh "storage-refresher" --revision 9

  	  wait_for "storage-refresher" "$(charm_rev "storage-refresher" 9)"

  	  juju add-unit storage-refresher
      wait_for "storage-refresher" "$(active_idle_condition "storage-refresher" 1)"

      # Assert the new unit has the at least 3072 MiB (same as revision 1).
      if ! assert_storage_min_size "storage-refresher/1" "awesome-fs/1" 3072 ; then
        echo $(red "attached storage is not at least 3072 in size")
        exit 1
      fi

      # Assert the new unit has the same mount location (same as revision 1).
      if ! assert_storage_mount_location "storage-refresher/1" "awesome-fs/1" "/awesome-fs" ; then
        echo $(red "awesome-fs/1 is not located in /awesome-fs")
        exit 1
      fi

      destroy_model "$model_name"
}

# Tests storage size decreases after charm refresh.
run_decrease_size() {
  	echo

  	model_name="test-decrease-size"
  	file="${TEST_DIR}/${model_name}.log"

    ensure "${model_name}" "${file}"

    juju deploy "storage-refresher" --revision 1 --channel latest/edge
    wait_for "storage-refresher" "$(active_idle_condition "storage-refresher")"

    # Assert that storage is attached to the unit.
    if ! assert_storage_attached "storage-refresher/0" "awesome-fs/0"; then
        echo $(red "awesome-fs/0 is not attached")
        exit 1
    fi

    # Refresh charm to revision 3 which has a lower storage size of 1G.
    juju refresh "storage-refresher" --revision 3

	  wait_for "storage-refresher" "$(charm_rev "storage-refresher" 3)"

	  juju add-unit storage-refresher
    wait_for "storage-refresher" "$(active_idle_condition "storage-refresher" 1)"

    # Assert the new unit has the new storage requirement.
    if ! assert_storage_min_size "storage-refresher/1" "awesome-fs/1" 1024 ; then
      echo $(red "attached storage is not at least 1024 in size")
      exit 1
    fi

    destroy_model "$model_name"
}

# Tests storage size increase is rejected during charm refresh.
run_increase_size() {
  	echo

  	model_name="test-increase-size"
  	file="${TEST_DIR}/${model_name}.log"

    ensure "${model_name}" "${file}"

    juju deploy "storage-refresher" --revision 1 --channel latest/edge
    wait_for "storage-refresher" "$(active_idle_condition "storage-refresher")"

    # Assert that storage is attached to the unit.
    if ! assert_storage_attached "storage-refresher/0" "awesome-fs/0"; then
        echo $(red "awesome-fs/0 is not attached")
        exit 1
    fi

    # Refresh charm to revision 2 which has a larger storage size.
    # This should fail.
    OUT=$(juju refresh "storage-refresher" --revision 2 2>&1 || true)
    echo "$OUT" | grep -q 'storage definition "awesome-fs".*exceeds existing minimum size'

    # Stay at revision 1.
    wait_for "storage-refresher" "$(charm_rev "storage-refresher" 1)"

    destroy_model "$model_name"
}

# Tests storage definition deletion is rejected during charm refresh.
run_delete_storage_definition() {
  	echo

  	model_name="test-delete-storage-definition"
  	file="${TEST_DIR}/${model_name}.log"

    ensure "${model_name}" "${file}"

    juju deploy "storage-refresher" --revision 1 --channel latest/edge
    wait_for "storage-refresher" "$(active_idle_condition "storage-refresher")"

    # Assert that storage is attached to the unit.
    if ! assert_storage_attached "storage-refresher/0" "awesome-fs/0"; then
        echo $(red "awesome-fs/0 is not attached")
        exit 1
    fi

    # Refresh charm to revision 5 which deletes storage "awesome-fs".
    # This should fail.
    OUT=$(juju refresh "storage-refresher" --revision 5 2>&1 || true)
    echo "$OUT" | grep -q 'storage definition "awesome-fs" removed'

    # Stay at revision 1.
    wait_for "storage-refresher" "$(charm_rev "storage-refresher" 1)"

    destroy_model "$model_name"
}

# Tests new storage definition is created for new units.
run_new_storage_definition() {
  	echo

  	model_name="test-new-storage-definition"
  	file="${TEST_DIR}/${model_name}.log"

    ensure "${model_name}" "${file}"

    juju deploy "storage-refresher" --revision 1 --channel latest/edge
    wait_for "storage-refresher" "$(active_idle_condition "storage-refresher")"

    # Assert that storage is attached to the unit.
    if ! assert_storage_attached "storage-refresher/0" "awesome-fs/0"; then
        echo $(red "awesome-fs/0 is not attached")
        exit 1
    fi

    # Refresh charm to revision 4 which adds a new storage definition "epic-fs".
    juju refresh "storage-refresher" --revision 4

	  wait_for "storage-refresher" "$(charm_rev "storage-refresher" 4)"

	  juju add-unit storage-refresher
    wait_for "storage-refresher" "$(active_idle_condition "storage-refresher" 1)"

    # Assert the new unit has the new storage requirement.
    if ! assert_storage_attached "storage-refresher/1" "awesome-fs/1"; then
        echo $(red "awesome-fs/1 is not attached")
        exit 1
    fi
    if ! assert_storage_attached "storage-refresher/1" "epic-fs/2"; then
        echo $(red "epic-fs/2 is not attached")
        exit 1
    fi

    destroy_model "$model_name"
}

# Tests a charm with single storage instance refreshed to one with
# multiple storage instance of the same name is rejected during charm refresh.
run_single_to_multiple_storage_instances_violates() {
  	echo

  	model_name="test-single-to-multiple-storage-instances-violates"
  	file="${TEST_DIR}/${model_name}.log"

    ensure "${model_name}" "${file}"

    juju deploy "storage-refresher" --revision 1 --channel latest/edge
    wait_for "storage-refresher" "$(active_idle_condition "storage-refresher")"

    # Assert that storage is attached to the unit.
    if ! assert_storage_attached "storage-refresher/0" "awesome-fs/0"; then
        echo $(red "awesome-fs/0 is not attached")
        exit 1
    fi

    # Refresh charm to revision 6 which changes "awesome-fs" to be multiple instances
    # with count range 2-5.
    # This should fail because it violates the existing storage requirement.
    OUT=$(juju refresh "storage-refresher" --revision 6 2>&1 || true)
    echo "$OUT" | grep -q 'storage definition "awesome-fs" new minimum count 2 exceeds existing minimum count 1'

    # Revision remains at 1.
	  wait_for "storage-refresher" "$(charm_rev "storage-refresher" 1)"

    destroy_model "$model_name"
}

# Tests changing a storage type from "filesystem" to "block" is rejected during
# charm refresh.
run_filesystem_to_block() {
  	echo

  	model_name="test-filesystem-to-block"
  	file="${TEST_DIR}/${model_name}.log"

    ensure "${model_name}" "${file}"

    juju deploy "storage-refresher" --revision 1 --channel latest/edge
    wait_for "storage-refresher" "$(active_idle_condition "storage-refresher")"

    # Assert that storage is attached to the unit.
    if ! assert_storage_attached "storage-refresher/0" "awesome-fs/0"; then
        echo $(red "awesome-fs/0 is not attached")
        exit 1
    fi

    # Refresh charm to revision 8 which changes "awesome-fs" to be a block type.
    # This should fail because you cannot change the storage type.
    OUT=$(juju refresh "storage-refresher" --revision 8 2>&1 || true)
    echo "$OUT" | grep -q 'storage definition "awesome-fs" type changed from "filesystem" to "block"'

    # Revision remains at 1.
    wait_for "storage-refresher" "$(charm_rev "storage-refresher" 1)"

    destroy_model "$model_name"
}

test_refresh_charm_storage() {
	if [ "$(skip 'test_refresh_charm_storage')" ]; then
		echo "==> TEST SKIPPED: refresh charm storage"
		return
	fi

	(
		set_verbosity

		cd .. || exit

    run "run_no_changes_in_new_revision"
    run "run_decrease_size"
    run "run_increase_size"
    run "run_delete_storage_definition"
    run "run_new_storage_definition"
    run "run_single_to_multiple_storage_instances_violates"
    run "run_filesystem_to_block"
	)
}
