run_refresh_new_storage_definition() {
	echo

	model_name="test-refresh-new-storage-definition"
	file="${TEST_DIR}/${model_name}.log"
	charm_name_to_refresh="testcharms/charms/refresh-storage-new-storage-def/refresh-storage.charm"

	ensure "${model_name}" "${file}"

	juju deploy "testcharms/charms/refresh-storage/refresh-storage.charm"
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

  # TODO: check that new units have "cool-fs" and "awesome-fs" storage attached and that old
  # unit only have "awesome-fs"
  juju status --storage

	destroy_model "${model_name}"
}

run_refresh_delete_storage_definition() {
	echo

	model_name="test-refresh-new-storage-definition"
	file="${TEST_DIR}/${model_name}.log"
	charm_name_to_refresh="testcharms/charms/refresh-storage-delete-storage-def/refresh-storage.charm"

	ensure "${model_name}" "${file}"

	juju deploy "testcharms/charms/refresh-storage/refresh-storage.charm"
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

  # TODO: check that new units have "cool-fs" storage attached and that old
  # unit have "awesome-fs" and "cool-fs"
  juju status --storage

	destroy_model "${model_name}"
}
