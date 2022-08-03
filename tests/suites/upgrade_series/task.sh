test_upgrade_series() {
	if [ "$(skip 'test_upgrade_series')" ]; then
		echo "==> TEST SKIPPED: upgrade series tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	test_upgrade_series_relation
}
