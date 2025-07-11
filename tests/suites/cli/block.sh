run_block_destroy_model() {
	# Echo out to ensure nice output to the test suite.
	echo

	model_name="test-block-destroy-model"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju disable-command destroy-model
	juju destroy-model --no-prompt ${model_name} | grep -q 'the operation has been blocked' || true

	juju enable-command destroy-model
	destroy_model "${model_name}"
}

run_block_remove_object() {
	# Echo out to ensure nice output to the test suite.
	echo

	model_name="test-block-remove-object"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy ubuntu-lite --base ubuntu@20.04 ubuntu
	juju deploy ntp
	juju integrate ntp ubuntu

	juju disable-command remove-object

	# juju status should still work when 'remove-object' commands
	# are disabled.
	wait_for "ntp" "$(idle_subordinate_condition "ntp" "ubuntu" 0)"

	juju destroy-model --no-prompt ${model_name} | grep -q 'the operation has been blocked' || true
	juju remove-application ntp | grep -q 'the operation has been blocked' || true
	juju remove-relation ntp ubuntu | grep -q 'the operation has been blocked' || true
	juju remove-unit ubuntu/0 | grep -q 'the operation has been blocked' || true

	juju enable-command remove-object

	juju remove-relation ntp ubuntu
	juju remove-application ntp
	juju remove-unit ubuntu/0

	destroy_model "${model_name}"
}

run_block_all() {
	# Echo out to ensure nice output to the test suite.
	echo

	model_name="test-block-all"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy ubuntu-lite --base ubuntu@20.04 ubuntu
	juju expose ubuntu

	juju disable-command all

	# juju status and offers should still work when 'all' commands
	# are disabled.
	juju status --format json | jq '.applications | .["ubuntu"] | .exposed' | check true
	juju offers | grep -q 'Offer' || true

	juju deploy ntp | grep -q 'the operation has been blocked' || true
	juju integrate ntp ubuntu | grep -q 'the operation has been blocked' || true
	juju unexpose ubuntu | grep -q 'the operation has been blocked' || true

	juju enable-command all

	juju deploy ntp
	juju integrate ntp ubuntu

	wait_for "ntp" "$(idle_subordinate_condition "ntp" "ubuntu" 0)"

	juju unexpose ubuntu
	juju status --format json | jq '.applications | .["ubuntu"] | .exposed' | check false

	destroy_model "${model_name}"
}

test_block_commands() {
	if [ "$(skip 'test_block_commands')" ]; then
		echo "==> TEST SKIPPED: block commands"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_block_destroy_model"
		run "run_block_remove_object"
		run "run_block_all"
	)
}
