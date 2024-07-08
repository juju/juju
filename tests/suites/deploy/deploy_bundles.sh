# NOTE: when making changes, remember that all the tests here need to be able
# to run on amd64 AND arm64.

run_deploy_bundle() {
	echo

	file="${TEST_DIR}/test-bundles-deploy.log"

	ensure "test-bundles-deploy" "${file}"

	juju deploy juju-qa-bundle-test
	wait_for "juju-qa-test" ".applications | keys[0]"
	wait_for "ntp" "$(idle_subordinate_condition "ntp" "juju-qa-test")"

	destroy_model "test-bundles-deploy"
}

run_deploy_bundle_overlay() {
	echo

	file="${TEST_DIR}/test-bundles-deploy-overlay.log"

	ensure "test-bundles-deploy-overlay" "${file}"

	bundle=./tests/suites/deploy/bundles/overlay_bundle.yaml
	juju deploy ${bundle}

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test" 0 0)"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test" 0 1)"

	destroy_model "test-bundles-deploy-overlay"
}

run_deploy_bundle_overlay_with_image_id() {
	echo
	echo "$ami_id"

	file="${TEST_DIR}/test-bundles-deploy-overlay-image-id.log"

	ensure "test-bundles-deploy-overlay-image-id" "${file}"

	bundle=./tests/suites/deploy/bundles/overlay_bundle_image_id.yaml
	sed "s/{{IMAGE_ID}}/${ami_id}/g" "${bundle}" >"${TEST_DIR}/overlay_bundle_image_id.yaml"

	juju deploy "${TEST_DIR}/overlay_bundle_image_id.yaml"

	wait_for "ubuntu" "$(idle_condition "ubuntu" 0 0)"
	wait_for "ubuntu" "$(idle_condition "ubuntu" 0 1)"

	destroy_model "test-bundles-deploy-overlay-image-id"
}

run_deploy_bundle_overlay_with_image_id_on_base_bundle() {
	echo

	file="${TEST_DIR}/test-bundles-deploy-overlay-image-id-on-base-bundle.log"

	ensure "test-bundles-deploy-overlay-image-id-on-base-bundle" "${file}"

	bundle=./tests/suites/deploy/bundles/overlay_bundle_image_id_on_base_bundle.yaml
	sed "s/{{IMAGE_ID}}/${ami_id}/g" "${bundle}" >"${TEST_DIR}/overlay_bundle_image_id_on_base_bundle.yaml"

	got=$(juju deploy "${TEST_DIR}/overlay_bundle_image_id_on_base_bundle.yaml" 2>&1 || true)
	check_contains "${got}" "'image-id' constraint in a base bundle not supported"

	destroy_model "test-bundles-deploy-overlay-image-id-on-base-bundle"
}

run_deploy_bundle_overlay_with_image_id_no_base() {
	echo

	file="${TEST_DIR}/test-bundles-deploy-overlay-image-id-no-base.log"

	ensure "test-bundles-deploy-overlay-image-id-no-base" "${file}"

	bundle=./tests/suites/deploy/bundles/overlay_bundle_image_id_no_base.yaml
	sed "s/{{IMAGE_ID}}/${ami_id}/g" "${bundle}" >"${TEST_DIR}/overlay_bundle_image_id_no_base.yaml"

	got=$(juju deploy "${TEST_DIR}/overlay_bundle_image_id_no_base.yaml" 2>&1 || true)
	check_contains "${got}" 'base must be explicitly provided for "ubuntu" when image-id constraint is used'

	destroy_model "test-bundles-deploy-overlay-image-id_no_base"
}

run_deploy_cmr_bundle() {
	echo

	file="${TEST_DIR}/test-cmr-bundles-deploy.log"

	ensure "test-cmr-bundles-deploy" "${file}"

	juju deploy juju-qa-dummy-source
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	juju add-model other
	juju switch other

	bundle=./tests/suites/deploy/bundles/cmr_bundles_test_deploy.yaml
	sed "s/{{BOOTSTRAPPED_JUJU_CTRL_NAME}}/${BOOTSTRAPPED_JUJU_CTRL_NAME}/g" "${bundle}" >"${TEST_DIR}/cmr_bundles_test_deploy.yaml"
	juju deploy "${TEST_DIR}/cmr_bundles_test_deploy.yaml"

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju switch "test-cmr-bundles-deploy"
	juju config dummy-source token=yeah-boi
	juju switch "other"
	wait_for "active" '."application-endpoints"["dummy-source"]."application-status".current'

	# TODO: no need to remove-relation before destroying model once we fixed(lp:1952221).
	juju remove-relation dummy-sink dummy-source
	# wait for relation removed.
	wait_for null '.applications["dummy-sink"] | .relations.source[0]'
	# The offer must be removed before model/controller destruction will work.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju switch "test-cmr-bundles-deploy"
	juju remove-offer "admin/test-cmr-bundles-deploy.dummy-source" -y

	destroy_model "other"
	destroy_model "test-cmr-bundles-deploy"
}

# run_deploy_exported_charmhub_bundle_with_fixed_revisions tests how juju deploys
# a charmhub bundle that specifies revisions for its charms
run_deploy_exported_charmhub_bundle_with_fixed_revisions() {
	echo

	file="${TEST_DIR}/test-export-bundles-deploy-with-fixed-revisions.log"

	ensure "test-export-bundles-deploy-with-fixed-revisions" "${file}"

	bundle=./tests/suites/deploy/bundles/telegraf_bundle.yaml
	juju deploy ${bundle}

	echo "Make a copy of reference yaml"
	cp ${bundle} "${TEST_DIR}/telegraf_bundle.yaml"
	if [[ -n ${MODEL_ARCH} ]]; then
		yq -yi "
			.machines.\"0\".constraints = \"arch=${MODEL_ARCH}\" |
			.machines.\"1\".constraints = \"arch=${MODEL_ARCH}\"
		" "${TEST_DIR}/telegraf_bundle.yaml"
	else
		yq -yi . "${TEST_DIR}/telegraf_bundle.yaml"
	fi
	# no need to wait for the bundle to finish deploying to
	# check the export.
	echo "Compare export-bundle with telegraf_bundle"
	juju export-bundle --filename "${TEST_DIR}/exported_bundle.yaml"
	# Sort keys in both yaml files to get a fair comparison.
	yq -yi --sort-keys '..' "${TEST_DIR}/telegraf_bundle.yaml"
	yq -yi --sort-keys '..' "${TEST_DIR}/exported_bundle.yaml"
	diff -u "${TEST_DIR}/telegraf_bundle.yaml" "${TEST_DIR}/exported_bundle.yaml"

	destroy_model "test-export-bundles-deploy-with-fixed-revisions"
}

# run_deploy_exported_charmhub_bundle_with_float_revisions checks how juju
# deploys a charmhub bundle when the revisions are not pinned
run_deploy_exported_charmhub_bundle_with_float_revisions() {
	echo

	file="${TEST_DIR}/test-export-bundles-deploy-with-float-revisions.log"

	ensure "test-export-bundles-deploy-with-float-revisions" "${file}"
	bundle=./tests/suites/deploy/bundles/telegraf_bundle_without_revisions.yaml
	bundle_with_fake_revisions=./tests/suites/deploy/bundles/telegraf_bundle_with_fake_revisions.yaml
	cp ${bundle} "${TEST_DIR}/telegraf_bundle_without_revisions.yaml"
	cp ${bundle_with_fake_revisions} "${TEST_DIR}/telegraf_bundle_with_fake_revisions.yaml"
	if [[ -n ${MODEL_ARCH} ]]; then
		yq -yi "
      .applications.influxdb.constraints = \"arch=${MODEL_ARCH}\" |
      .applications.juju-qa-test.constraints = \"arch=${MODEL_ARCH}\"
    " "${TEST_DIR}/telegraf_bundle_without_revisions.yaml"
		yq -yi "
      .applications.influxdb.constraints = \"arch=${MODEL_ARCH}\" |
      .applications.juju-qa-test.constraints = \"arch=${MODEL_ARCH}\"
    " "${TEST_DIR}/telegraf_bundle_with_fake_revisions.yaml"
	fi

	# Add correct PGP key for influxdb - workaround from
	# https://bugs.launchpad.net/influxdb-charm/+bug/2004303
	INFLUXDB_PGP=$(curl -s https://repos.influxdata.com/influxdata-archive_compat.key)
	yq -yi "
		.applications.influxdb.options.install_keys = \"${INFLUXDB_PGP}\"
	" "${TEST_DIR}/telegraf_bundle_without_revisions.yaml"
	yq -yi "
		.applications.influxdb.options.install_keys = \"${INFLUXDB_PGP}\"
	" "${TEST_DIR}/telegraf_bundle_with_fake_revisions.yaml"

	juju deploy "${TEST_DIR}/telegraf_bundle_without_revisions.yaml"

	echo "Create telegraf_bundle_without_revisions.yaml with known latest revisions from charmhub"
	if [[ -n ${MODEL_ARCH} ]]; then
		influxdb_rev=$(juju info influxdb --arch="${MODEL_ARCH}" --format json | jq -r '."channels"."latest"."stable"[0].revision')
		telegraf_rev=$(juju info telegraf --arch="${MODEL_ARCH}" --format json | jq -r '."channels"."latest"."stable"[0].revision')
		juju_qa_test_rev=$(juju info juju-qa-test --arch="${MODEL_ARCH}" --format json | jq -r '."channels"."latest"."candidate"[0].revision')
	else
		influxdb_rev=$(juju info influxdb --format json | jq -r '."channels"."latest"."stable"[0].revision')
		telegraf_rev=$(juju info telegraf --format json | jq -r '."channels"."latest"."stable"[0].revision')
		juju_qa_test_rev=$(juju info juju-qa-test --format json | jq -r '."channels"."latest"."candidate"[0].revision')
	fi

	echo "Make a copy of reference yaml and insert revisions in it"
	cp "${TEST_DIR}/telegraf_bundle_with_fake_revisions.yaml" "${TEST_DIR}/telegraf_bundle_with_revisions.yaml"
	yq -yi "
		.applications.influxdb.revision = ${influxdb_rev} |
		.applications.telegraf.revision = ${telegraf_rev} |
		.applications.juju-qa-test.revision = ${juju_qa_test_rev}
	" "${TEST_DIR}/telegraf_bundle_with_revisions.yaml"

	if [[ -n ${MODEL_ARCH} ]]; then
		yq -yi "
			.applications.influxdb.constraints = \"arch=${MODEL_ARCH}\" |
			.applications.juju-qa-test.constraints = \"arch=${MODEL_ARCH}\" |
			.machines.\"0\".constraints = \"arch=${MODEL_ARCH}\" |
			.machines.\"1\".constraints = \"arch=${MODEL_ARCH}\"
		" "${TEST_DIR}/telegraf_bundle_with_revisions.yaml"
	fi

	# The model should be updated immediately, so we can export the bundle before
	# everything is done deploying
	echo "Compare export-bundle with telegraf_bundle_with_revisions"
	juju export-bundle --filename "${TEST_DIR}/exported_bundle.yaml"
	# Sort keys in both yaml files to get a fair comparison.
	yq -yi --sort-keys '..' "${TEST_DIR}/telegraf_bundle_with_revisions.yaml"
	yq -yi --sort-keys '..' "${TEST_DIR}/exported_bundle.yaml"
	diff -u "${TEST_DIR}/telegraf_bundle_with_revisions.yaml" "${TEST_DIR}/exported_bundle.yaml"

	destroy_model "test-export-bundles-deploy-with-float-revisions"
}

run_deploy_trusted_bundle() {
	echo

	file="${TEST_DIR}/test-trusted-bundles-deploy.log"

	ensure "test-trusted-bundles-deploy" "${file}"

	bundle=./tests/suites/deploy/bundles/trusted_bundle.yaml
	OUT=$(juju deploy ${bundle} 2>&1 || true)
	echo "${OUT}" | check "repeat the deploy command with the --trust argument"

	juju deploy --trust ${bundle}

	wait_for "trust-checker" "$(idle_condition "trust-checker")"

	destroy_model "test-trusted-bundles-deploy"
}

run_deploy_charmhub_bundle() {
	echo

	model_name="test-charmhub-bundle-deploy"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	bundle=juju-qa-bundle-test
	juju deploy "${bundle}"

	wait_for "juju-qa-test" "$(charm_channel "juju-qa-test" "2.0/stable")"
	wait_for "juju-qa-test-focal" "$(charm_channel "juju-qa-test-focal" "latest/candidate")"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	wait_for "juju-qa-test-focal" "$(idle_condition "juju-qa-test-focal" 1)"
	wait_for "ntp" "$(idle_subordinate_condition "ntp" "juju-qa-test")"
	wait_for "ntp-focal" "$(idle_subordinate_condition "ntp-focal" "juju-qa-test-focal")"

	destroy_model "${model_name}"
}

# run_deploy_lxd_profile_bundle is to deploy multiple units of the
# same charm which has an lxdprofile in a bundle.  The scenario
# created by the bundle was found to produce failure cases during
# development of the lxd profile feature.
run_deploy_lxd_profile_bundle() {
	echo

	model_name="test-deploy-lxd-profile-bundle"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	bundle=./tests/suites/deploy/bundles/lxd-profile-bundle.yaml
	juju deploy "${bundle}"

	# 8 units of lxd-profile
	for i in 0 1 2 3 4 5 6 7; do
		wait_for "lxd-profile" "$(idle_condition "lxd-profile" 0 "${i}")"
	done
	# 4 units of ubuntu
	for i in 0 1 2 3; do
		wait_for "ubuntu" "$(idle_condition "ubuntu" 1 "${i}")"
	done

	lxd_profile_name="juju-${model_name}-lxd-profile"
	for i in 0 1 2 3; do
		machine_n_lxd0="$(machine_container_path "${i}" "${i}"/lxd/0)"
		juju status --format=json | jq "${machine_n_lxd0}" | check "${lxd_profile_name}"
		machine_n_lxd1="$(machine_container_path "${i}" "${i}"/lxd/1)"
		juju status --format=json | jq "${machine_n_lxd1}" | check "${lxd_profile_name}"
	done

	destroy_model "${model_name}"
}

# run_deploy_multi_app_single_charm_bundle:
# LP 1999060 found an issue in async charm download when a bundle
# uses the same charm for multiple applications. This is common in
# many Canonical bundles such a full openstack.
run_deploy_multi_app_single_charm_bundle() {
	echo

	model_name="test-deploy-multi-app-single-charm-bundle"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	bundle=./tests/suites/deploy/bundles/multi-app-single-charm.yaml
	juju deploy "${bundle}"

	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test" 0)"
	wait_for "juju-qa-test-dup" "$(idle_condition "juju-qa-test-dup" 1)"

	# ensure juju-qa-test-dup can refresh and us it's resources.
	juju refresh juju-qa-test-dup

	destroy_model "${model_name}"
}

test_deploy_bundles() {
	if [ "$(skip 'test_deploy_bundles')" ]; then
		echo "==> TEST SKIPPED: deploy bundles"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_bundle"
		run "run_deploy_bundle_overlay"
		run "run_deploy_exported_charmhub_bundle_with_fixed_revisions"
		run "run_deploy_exported_charmhub_bundle_with_float_revisions"
		run "run_deploy_trusted_bundle"
		run "run_deploy_charmhub_bundle"
		run "run_deploy_multi_app_single_charm_bundle"

		# LXD specific profile tests.
		case "${BOOTSTRAP_PROVIDER:-}" in
		"lxd")
			echo "==> TEST SKIPPED: deploy_lxd_profile_bundle - tests for non LXD only"
			;;
		*)
			run "run_deploy_lxd_profile_bundle"
			;;
		esac

		# AWS specific image id tests.
		case "${BOOTSTRAP_PROVIDER:-}" in
		"ec2")
			check_dependencies aws
			add_clean_func "run_cleanup_ami"
			export ami_id
			create_ami_and_wait_available "ami_id"

			run "run_deploy_bundle_overlay_with_image_id"
			run "run_deploy_bundle_overlay_with_image_id_on_base_bundle"
			run "run_deploy_bundle_overlay_with_image_id_no_base"
			;;
		*)
			echo "==> TEST SKIPPED: deploy_bundle_with_image_id - tests for AWS only"
			echo "==> TEST SKIPPED: deploy_bundle_with_image_id_on_base_bundle - tests for AWS only"
			echo "==> TEST SKIPPED: deploy_bundle_with_image_id_no_base - tests for AWS only"
			;;
		esac

		# Run this last so the other tests run, there are intermittent issues
		# in cmr tear down.
		run "run_deploy_cmr_bundle"
	)
}
