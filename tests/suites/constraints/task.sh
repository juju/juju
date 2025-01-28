test_constraints() {
	if [ "$(skip 'test_constraints')" ]; then
		echo "==> TEST SKIPPED: constraints tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	test_constraints_common

	test_constraints_model
}
