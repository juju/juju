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
	juju models --format json | jq -r '."current-model"' | check 'model-destroy'

	echo "Add new model 'model-new'"
	juju add-model model-new

	echo "Ensure current model is 'model-new'"
	juju models --format json | jq -r '."current-model"' | check 'model-new'

	echo "Destroy model 'model-new'"
	juju destroy-model --no-prompt 'model-new'

	echo "Ensure model 'model-new' is destroyed"
	is_destroyed=$(juju models --format json | jq -r '.models[] | select(."short-name" == "model-new")')
	if [[ -z ${is_destroyed} ]]; then is_destroyed=true; fi
	check_contains "${is_destroyed}" true

	echo "Switch to model 'model-destroy'"
	juju switch model-destroy

	echo "Ensure current model is 'model-destroy'"
	juju models --format json | jq -r '."current-model"' | check 'model-destroy'

	destroy_model "model-destroy"
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
	)
}
