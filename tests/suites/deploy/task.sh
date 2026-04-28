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
		_test_deploy_parallel
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

# _test_deploy_parallel runs deploy tests concurrently using JUJU_DATA
# isolation. Tests that require exclusive controller state (e.g. juju switch
# across models) run sequentially afterward.
_test_deploy_parallel() {
	(
		set_verbosity

		cd .. || exit

		echo "==> Checking for dependencies"
		check_dependencies charmcraft

		# -- Charm tests --
		run_parallel "run_deploy_charm"
		run_parallel "run_deploy_specific_series"
		run_parallel "run_resolve_charm"
		run_parallel "run_deploy_charm_unsupported_series"

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			if kvm-ok; then
				run_parallel "run_deploy_charm_placement_directive"
			else
				echo "==> TEST SKIPPED: deploy_charm_placement_directive - lxd without kvm is not supported"
			fi
			run_parallel "run_deploy_local_predeployed_charm"
			echo "==> TEST SKIPPED: deploy_lxd_to_container - tests for non LXD only"
			echo "==> TEST SKIPPED: deploy_lxd_profile_charm_container - tests for non LXD only"
			;;
		*)
			run_parallel "run_deploy_charm_placement_directive"
			run_parallel "run_deploy_lxd_to_container"
			run_parallel "run_deploy_lxd_profile_charm_container"
			;;
		esac

		# -- Bundle tests (CMR bundle excluded — uses juju switch) --
		run_parallel "run_deploy_bundle"
		run_parallel "run_deploy_bundle_overlay"
		run_parallel "run_deploy_trusted_bundle"
		run_parallel "run_deploy_multi_app_single_charm_bundle"

		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			echo "==> TEST SKIPPED: deploy_lxd_profile_bundle - tests for non LXD only"
			;;
		"ec2")
			setup_awscli_credential
			check_dependencies aws
			add_clean_func "run_cleanup_ami"
			export ami_id
			create_ami_and_wait_available "ami_id"

			run_parallel "run_deploy_bundle_overlay_with_image_id"
			run_parallel "run_deploy_bundle_overlay_with_image_id_on_base_bundle"
			run_parallel "run_deploy_bundle_overlay_with_image_id_no_base"
			;;
		*)
			echo "==> TEST SKIPPED: deploy_lxd_profile_bundle - tests for non LXD only"
			echo "==> TEST SKIPPED: deploy_bundle_with_image_id - tests for AWS only"
			echo "==> TEST SKIPPED: deploy_bundle_with_image_id_on_base_bundle - tests for AWS only"
			echo "==> TEST SKIPPED: deploy_bundle_with_image_id_no_base - tests for AWS only"
			;;
		esac

		# -- Revision tests --
		run_parallel "run_deploy_revision"
		run_parallel "run_deploy_revision_fail"
		run_parallel "run_deploy_revision_refresh"
		run_parallel "run_deploy_revision_resource"

		# -- Default base tests --
		run_parallel "run_deploy_default_series"
		run_parallel "run_deploy_not_default_series"

		# Execute all queued tests concurrently.
		wait_parallel
	)

	# CMR bundle uses 'juju switch' across models — run sequentially.
	(
		set_verbosity

		cd .. || exit

		run "run_deploy_cmr_bundle"
	)

	# export-overlay tests (currently disabled upstream).
	test_cmr_bundles_export_overlay
}
