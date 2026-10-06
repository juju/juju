# Tests if Juju tracks the model properly through deletion.
#
# In normal behavior Juju should drop the current model selection if that
# model is destroyed. This will fail if Juju does not drop it's current
# selection.
run_model_destroy() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-model-destroy.log"
	ensure "model-destroy" "${file}"

	echo "Ensure current model is 'model-destroy'"
	juju models --format json | yq -r '."current-model"' | check 'model-destroy'

	echo "Add new model 'model-new'"
	juju add-model model-new

	echo "Ensure current model is 'model-new'"
	juju models --format json | yq -r '."current-model"' | check 'model-new'

	echo "Destroy model 'model-new'"
	juju destroy-model --no-prompt 'model-new'

	echo "Ensure model 'model-new' is destroyed"
	is_destroyed=$(juju models --format json | yq -r '.models[] | select(."short-name" == "model-new")')
	if [[ -z ${is_destroyed} ]]; then is_destroyed=true; fi
	check_contains "${is_destroyed}" true

	echo "Switch to model 'model-destroy'"
	juju switch model-destroy

	echo "Ensure current model is 'model-destroy'"
	juju models --format json | yq -r '."current-model"' | check 'model-destroy'

	destroy_model "model-destroy"
}

# Tests that destroying a model hosting an LXD container completes: the
# container host must get a removal job and be deleted once its container
# is gone (JUJU-10360). The framework's teardown deliberately swallows
# destroy errors, so this test asserts the model's disappearance itself
# and fails (rather than warn) if the teardown hangs.
run_model_destroy_with_container() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures a bootstrap juju exists.
	file="${TEST_DIR}/test-model-destroy-container.log"
	ensure "model-destroy-container" "${file}"

	echo "Add machine 0 and LXD container 0/lxd/0"
	# Container in container doesn't work consistently enough for the
	# test, so use a virtual machine host on the lxd provider.
	if [[ ${BOOTSTRAP_PROVIDER} == "lxd" ]]; then
		juju add-machine --constraints="virt-type=virtual-machine"
	else
		juju add-machine
	fi
	juju add-machine lxd:0
	# If 0/lxd/0 is started, so must machine 0 be.
	wait_for_container_agent_status "0/lxd/0" "started"

	echo "Destroy model 'model-destroy-container'"
	output="${TEST_DIR}/model-destroy-container-destroy.log"
	timeout "${DESTROY_TIMEOUT}" juju destroy-model --no-prompt 'model-destroy-container' >"${output}" 2>&1 || true

	echo "Ensure model 'model-destroy-container' is destroyed"
	is_destroyed=$(juju models --format json | yq -r '.models[] | select(."short-name" == "model-destroy-container")')
	if [[ -n ${is_destroyed} ]]; then
		echo "FAIL: model 'model-destroy-container' still exists after destroy; see ${output}"
		return 1
	fi
}

test_model_destroy() {
	if [ -n "$(skip 'test_model_destroy')" ]; then
		echo "==> SKIP: Asked to skip model destroy tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_destroy"
		run "run_model_destroy_with_container"
	)
}
