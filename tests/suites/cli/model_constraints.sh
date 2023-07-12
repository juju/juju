run_model_constraints() {
	echo

	juju set-model-constraints "cores=2 mem=6G"
	juju model-constraints | check "cores=2 mem=6144M"
}

test_model_constraints() {
	if [ "$(skip 'test_model_constraints')" ]; then
		echo "==> TEST SKIPPED: model constraints"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_constraints"
	)
}
