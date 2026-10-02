test_modelmigration() {
	if [ "$(skip 'test_modelmigration')" ]; then
		echo "==> TEST SKIPPED: model migration tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju juju_36

	test_modelmigration_relation_egress_override
}
