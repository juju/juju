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

	if [[ ${TEST_PARALLEL:-} == "true" ]]; then
		# Override `run` so that existing group functions queue tests
		# instead of executing them sequentially. The file-based queue
		# survives the subshell boundaries used by each group function.
		parallel_init
		# shellcheck disable=SC2329
		run() { run_parallel "$@"; }

		test_deploy_charms
		test_deploy_bundles
		test_cmr_bundles_export_overlay
		test_deploy_revision
		test_deploy_default_series

		unset -f run
		wait_parallel
	else
		test_deploy_charms
		test_deploy_bundles
		test_cmr_bundles_export_overlay
		test_deploy_revision
		test_deploy_default_series
	fi

	if [[ ${skip_bootstrap} != "true" ]]; then
		destroy_controller "test-deploy-ctl"
	fi
}
