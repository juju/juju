run_refresh_new_storage_definition() {
	echo

	model_name="test-refresh-new-storage-definition"
	file="${TEST_DIR}/${model_name}.log"
	charm_name_to_refresh="./testcharms/charms/refresh-storage-new-storage-def/refresh-storage.charm"

	ensure "${model_name}" "${file}"

	juju deploy "./testcharms/charms/refresh-storage/refresh-storage.charm"
	wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

	OUT=$(juju refresh refresh-storage --path "${charm_name_to_refresh}" 2>&1 || true)
	if echo "${OUT}" | grep -v "no change" | grep -E -vq "Added local charm"; then
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	printf "${OUT}\n"

	wait_for "refresh-storage" "$(charm_rev "refresh-storage" 1)"
	wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

	juju add-unit refresh-storage
  wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

  # check there are 2 units
  num_of_units=$(juju status --format json | jq '.applications["refresh-storage"].units | to_entries | length')

  if [ "$num_of_units" -ne 2 ]; then
		echo $(red "expected 2 units, obtained $num_of_units")
		exit 1
  fi

  storages=$(juju status --format json | jq '.storage.storage')
  # there must be 3 storages in total after adding a new unit
  num_of_storages=$(echo $storages | jq 'to_entries | length')
  if [ "$num_of_storages" -ne 3 ]; then
      echo $(red "expected 3 storages, obtained $num_of_storages")
      		exit 1
  fi

  # old unit "refresh-storage/0" only has "awesome-fs" storage
  if ! echo $storages | jq -e '
    .["awesome-fs/0"].attachments.units | to_entries | length == 1 and .[0].key == "refresh-storage/0"
  ' >/dev/null; then
    echo $(red "expected awesome-fs/0 storage to only be attached to refresh-storage/0 unit")
  fi

  #  check that new unit "refresh-storage/1" have "cool-fs" and "awesome-fs" storage attached
  if ! echo $storages | jq -e '
  .["awesome-fs/1"].attachments.units | to_entries | length == 1 and .[0].key == "refresh-storage/1"
  ' >/dev/null; then
    echo $(red "expected awesome-fs/1 storage to only be attached to refresh-storage/1 unit")
  fi

  if ! echo $storage | jq -e '
  .["cool-fs/2"].attachments.units | to_entries | length == 1 and .[0].key == "refresh-storage/1"
  ' >/dev/null; then
    echo $(red "expected cool-fs/2 storage to only be attached to refresh-storage/1 unit")
  fi

	destroy_model "${model_name}"
}

run_refresh_delete_storage_definition() {
	echo

	model_name="test-refresh-delete-storage-definition"
	file="${TEST_DIR}/${model_name}.log"
	charm_name_to_refresh="./testcharms/charms/refresh-storage-delete-storage-def/refresh-storage.charm"

	ensure "${model_name}" "${file}"

	juju deploy "./testcharms/charms/refresh-storage/refresh-storage.charm"
	wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

	OUT=$(juju refresh refresh-storage --path "${charm_name_to_refresh}" 2>&1 || true)
	if echo "${OUT}" | grep -v "no change" | grep -E -vq "Added local charm"; then
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	printf "${OUT}\n"

	wait_for "refresh-storage" "$(charm_rev "refresh-storage" 1)"
	wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

	juju add-unit refresh-storage
  wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

  # check there are 2 units.
  num_of_units=$(juju status --format json | jq '.applications["refresh-storage"].units | to_entries | length')

  if [ "$num_of_units" -ne 2 ]; then
    echo $(red "expected 2 units, obtained $num_of_units")
    exit 1
  fi

  storages=$(juju status --format json | jq '.storage.storage')
  # there must be 2 storages in total after adding a new unit.
  # the new unit is attached 1 storage and the old unit remains on the old storage.
  num_of_storages=$(echo $storages | jq 'to_entries | length')
  if [ "$num_of_storages" -ne 2 ]; then
      echo $(red "expected 2 storages, obtained $num_of_storages")
      exit 1
  fi

  # old unit "refresh-storage/0" only has "awesome-fs" storage
  if ! echo $storages | jq -e '
    .["awesome-fs/0"].attachments.units | to_entries | length == 1 and .[0].key == "refresh-storage/0"
  ' >/dev/null; then
    echo $(red "expected awesome-fs/0 storage to only be attached to refresh-storage/0 unit")
  fi

  # check that new unit "refresh-storage/1" only has "cool-fs" storage attached
  if ! echo $storages | jq -e '
  .["cool-fs/1"].attachments.units | to_entries | length == 1 and .[0].key == "refresh-storage/1"
  ' >/dev/null; then
    echo $(red "expected cool-fs/1 storage to only be attached to refresh-storage/1 unit")
  fi


	destroy_model "${model_name}"
}

run_refresh_increase_max_count() {
	echo

	model_name="test-refresh-increase-max-count"
	file="${TEST_DIR}/${model_name}.log"
	charm_name_to_refresh="./testcharms/charms/refresh-storage-multiple-increase-max-count"

	ensure "${model_name}" "${file}"

	juju deploy $(pack_charm "./testcharms/charms/refresh-storage-multiple")
	wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

  # This fails because it violates the charm storage max count
  if ! juju add-storage refresh-storage/0 awesome-multi-fs="rootfs,10M,5" | grep -q "
    request count 6 exceeds the charm's maximum count of 5
  "; then
      echo $(red "expected to fail adding storage")
      exit 1
  fi

  # Refresh charm with a higher storage max count.
	OUT=$(juju refresh refresh-storage --path $(pack_charm ${charm_name_to_refresh}) 2>&1 || true)
	if echo "${OUT}" | grep -v "no change" | grep -E -vq "Added local charm"; then
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	printf "${OUT}\n"

	wait_for "refresh-storage" "$(charm_rev "refresh-storage" 1)"
	wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

  # Add a unit so that the new storage requirement is reflected in the new unit.
	juju add-unit refresh-storage
  wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

  # check there are 2 units.
  num_of_units=$(juju status --format json | jq '.applications["refresh-storage"].units | to_entries | length')

  if [ "$num_of_units" -ne 2 ]; then
    echo $(red "expected 2 units, obtained $num_of_units")
    exit 1
  fi

  # Add new storage with a higher count than before.
  add_storage_output_success=$(juju add-storage refresh-storage/1 awesome-multi-fs="rootfs,10M,6")
  if ! echo "$add_storage_output_success" | grep -q "REPLACE-ME-WITH-SUCCESS"; then
      echo $(red "expected to succeed adding storage")
      exit 1
  fi

  # This fails because the unit has reached the maximum count.
  if ! juju add-storage refresh-storage/1 awesome-multi-fs="rootfs,10M,5" | grep -q "REPLACE-ME-WITH-FAILURE"; then
      echo $(red "expected to fail adding storage")
      exit 1
  fi

  wait_for "refresh-storage" "$(active_idle_condition "refresh-storage")"

  storages=$(juju status --format json | jq '.storage.storage')
  # there must be 8 storages in total after adding a new unit.
  # the new unit is attached 7 storages and the old unit remains attached to 1 storage.
  num_of_storages=$(echo $storages | jq 'to_entries | length')
  if [ "$num_of_storages" -ne 7 ]; then
      echo $(red "expected 8 storages, obtained length: $num_of_storages, storages: $storages")
      exit 1
  fi

	destroy_model "${model_name}"
}

test_refresh_charm_storage() {
	if [ "$(skip 'test_refresh_charm_storage')" ]; then
		echo "==> TEST SKIPPED: refresh charm storage"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_refresh_new_storage_definition"
		run "run_refresh_delete_storage_definition"
	)
}
