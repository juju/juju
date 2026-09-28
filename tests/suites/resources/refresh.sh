run_resource_refresh() {
	echo
	name="resource-upgrade"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	juju deploy juju-qa-test --channel 2.0/edge
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	juju config juju-qa-test foo-file=true

	# wait for update-status
	wait_for "resource line one: testing one plus one." "$(workload_status juju-qa-test 0).message"
	juju config juju-qa-test foo-file=false

	juju refresh juju-qa-test --channel 2.0/stable
	wait_for "juju-qa-test" "$(charm_channel "juju-qa-test" "2.0/stable")"

	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing one." "$(workload_status juju-qa-test 0).message"

	destroy_model "test-${name}"
}

run_resource_refresh_no_new_charm_rev() {
	echo
	name="resource-upgrade-no-new-charm-rev"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	juju deploy juju-qa-test --resource foo-file=4
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	juju config juju-qa-test foo-file=true

	# wait for update-status
	wait_for "resource line one: testing four." "$(workload_status juju-qa-test 0).message"
	juju resources juju-qa-test --format json | yq '.resources[0] | .[ "revision"] == "4"'
	juju config juju-qa-test foo-file=false

	juju refresh juju-qa-test

	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing two." "$(workload_status juju-qa-test 0).message"
	juju resources juju-qa-test --format json | yq '.resources[0] | .[ "revision"] == "2"'

	destroy_model "test-${name}"
}

check_application_and_unit_resources() {
	local application=$1
	local resource_name=$2
	local expected_revision=$3
	local application_fingerprint unit_fingerprint

	application_fingerprint=$(juju list-resources "${application}" --format json |
		yq -r '.resources[] | select(.name == "'"${resource_name}"'") | .fingerprint')
	test -n "${application_fingerprint}"
	juju list-resources "${application}" --format json |
		yq -r '.resources[] | select(.name == "'"${resource_name}"'") | .revision' |
		check "^${expected_revision}$"

	for unit in 0 1; do
		unit_fingerprint=$(juju list-resources "${application}/${unit}" --format json |
			yq -r '.[] | select(.name == "'"${resource_name}"'") | .fingerprint')
		juju list-resources "${application}/${unit}" --format json |
			yq -r '.[] | select(.name == "'"${resource_name}"'") | .revision' |
			check "^${expected_revision}$"
		test "${unit_fingerprint}" = "${application_fingerprint}"
	done
}

run_resource_refresh_no_new_charm_rev_supply_res_rev() {
	# refresh the resource revision without changing the
	# charm url
	echo
	name="resource-refresh-no-new-charm-rev-supply-res-rev"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	juju charm-resources juju-qa-test --channel latest/stable --format json |
		yq -r '.[] | select(.name == "foo-file") | .revision' |
		check '^2$'

	juju deploy juju-qa-test --channel latest/stable -n 2
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
	juju config juju-qa-test foo-file=true

	# wait for update-status
	wait_for "resource line one: testing two." "$(workload_status juju-qa-test 0).message"
	wait_for "resource line one: testing two." "$(workload_status juju-qa-test 1).message"
	check_application_and_unit_resources juju-qa-test foo-file 2
	juju config juju-qa-test foo-file=false

	juju refresh juju-qa-test --resource foo-file=3

	juju config juju-qa-test foo-file=true
	wait_for "resource line one: testing one plus one." "$(workload_status juju-qa-test 0).message"
	wait_for "resource line one: testing one plus one." "$(workload_status juju-qa-test 1).message"
	check_application_and_unit_resources juju-qa-test foo-file 3

	destroy_model "test-${name}"
}

run_resource_no_upgrade_after_upload() {
	# Deploy with an uploaded resource. Verify the resource doesn't
	# change after refresh.
	echo
	name="resource-no-upgrade-after-upload"

	file="${TEST_DIR}/test-${name}.log"

	ensure "test-${name}" "${file}"

	juju deploy juju-qa-test --resource foo-file="./tests/suites/resources/foo-file.txt"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	juju config juju-qa-test foo-file=true
	# wait for config-changed, the charm will update the status
	# to include the contents of foo-file.txt
	wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"

	juju refresh juju-qa-test --channel 2.0/stable
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	# check resource revision hasn't changed.
	juju resources juju-qa-test --format json | yq '.resources[0] | .[ "revision"] == "0"'
	juju resources juju-qa-test --format json | yq '.resources[0] | .[ "origin"] == "upload"'

	destroy_model "test-${name}"
}

test_upgrade_resources() {
	if [ "$(skip 'test_upgrade_resources')" ]; then
		echo "==> TEST SKIPPED: Resource upgrades"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_resource_refresh"
		run "run_resource_refresh_no_new_charm_rev"
		run "run_resource_no_upgrade_after_upload"
		run "run_resource_refresh_no_new_charm_rev_supply_res_rev"
	)
}
