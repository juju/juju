test_deploy() {
	if [ "$(skip 'test_deploy')" ]; then
		echo "==> TEST SKIPPED: Deploy tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju

	file="${TEST_DIR}/test-deploy-ctl.log"

	# When running only this subtest, don't create/destroy a controller.
	skip_bootstrap=false
	if [[ ${RUN_LIST:-} =~ ^[^,]+,test_cmr_bundles_export_overlay$ ]] ; then
		skip_bootstrap=true
	fi

	if [[ ${skip_bootstrap} != "true" ]]; then
		bootstrap "test-deploy-ctl" "${file}"
	fi

	test_deploy_charms
	test_deploy_bundles
	test_cmr_bundles_export_overlay
	test_deploy_revision
	test_deploy_default_series

	if [[ ${skip_bootstrap} != "true" ]]; then
		destroy_controller "test-deploy-ctl"
	fi
}
