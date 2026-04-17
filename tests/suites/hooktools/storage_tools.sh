run_storage_list() {
	echo

	model_name="test-storage-list"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy juju-qa-dummy-storage x --storage single-fs=1 \
		--storage multi-fs=2
	wait_for "x" "$(idle_condition "x")"

	storage=$(juju exec --unit x/0 'storage-list')
	echo "$storage" | awk -F'/' '/^multi-fs/{count++} END{print count+0}' | check '2'
	echo "$storage" | awk -F'/' '/^single-fs/{count++} END{print count+0}' | check '1'

	destroy_model "${model_name}"
}

run_storage_get() {
	echo

	model_name="test-storage-get"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy juju-qa-dummy-storage x --storage single-fs=1
	wait_for "x" "$(idle_condition "x")"

	kind=$(juju exec --unit x/0 'storage-get -s single-fs/0 kind')
	echo $kind | check 'filesystem'
	
	location=$(juju exec --unit x/0 'storage-get -s single-fs/0 location')
	check_contains "$location" '/srv/single-fs'

	yaml=$(juju exec --unit x/0 'storage-get -s single-fs/0 --format=yaml')
	echo "$yaml" | yq -r '.kind' | check "$kind"
	echo "$yaml" | yq -r '.location' | check "$location"

	destroy_model "${model_name}"
}

run_storage_add() {
	echo

	model_name="test-storage-add"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy juju-qa-dummy-storage x
	wait_for "x" "$(idle_condition "x")"

	storage=$(juju exec --unit x/0 'storage-list')
	echo "$storage" | awk -F'/' '/^single-fs/{count++} END{print count+0}' | check '0'
	echo "$storage" | awk -F'/' '/^multi-fs/{count++} END{print count+0}' | check '0'
	
	juju exec --unit x/0 'storage-add single-fs'
	storage=$(juju exec --unit x/0 'storage-list')
	echo "$storage" | awk -F'/' '/^single-fs/{count++} END{print count+0}' | check '1'
	
	juju exec --unit x/0 'storage-add multi-fs=2'
	storage=$(juju exec --unit x/0 'storage-list')
	echo "$storage" | awk -F'/' '/^multi-fs/{count++} END{print count+0}' | check '2'

	destroy_model "${model_name}"
}

test_storage_hook_tools() {
	if [ "$(skip 'test_storage_hook_tools')" ]; then
		echo "==> TEST SKIPPED: storage hook tools"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_storage_get"
		run "run_storage_list"
		
		case "${BOOTSTRAP_PROVIDER:-}" in
		"k8s")
			echo "==> TEST SKIPPED: storage_add"
			;;
		*)
			run "run_storage_add"
			;;
		esac
	)
}
