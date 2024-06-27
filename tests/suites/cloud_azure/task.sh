test_cloud_azure() {
	if [ "$(skip 'test_cloud_azure')" ]; then
		echo "==> TEST SKIPPED: azure cloud"
		return
	fi

	set_verbosity

	if [ "${BOOTSTRAP_PROVIDER}" != "azure" ]; then
		echo "==> TEST SKIPPED: azure cloud tests, not using azure"
		return
	fi

	echo "==> Checking for dependencies"

	check_dependencies juju az jq

	test_managed_identity
}
