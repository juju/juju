run_refresh_local() {
	# Test a plain juju refresh with a local charm
	echo

	model_name="test-refresh-local"
	file="${TEST_DIR}/${model_name}.log"
	charm_name="${TEST_DIR}/ubuntu.charm"

	ensure "${model_name}" "${file}"

	juju download ubuntu --no-progress - >"${charm_name}"
	juju deploy "${charm_name}" ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	OUT=$(juju refresh ubuntu --path "${charm_name}" 2>&1 || true)
	if echo "${OUT}" | grep -E -vq "Added local charm"; then
		# shellcheck disable=SC2046
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	# shellcheck disable=SC2059
	printf "${OUT}\n"

	# format: Added charm-store charm "ubuntu", revision 21 in channel stable, to the model
	revision=$(echo "${OUT}" | awk 'BEGIN{FS=","} {print $2}' | awk 'BEGIN{FS=" "} {print $2}')

	wait_for "ubuntu" "$(charm_rev "ubuntu" "${revision}")"
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	destroy_model "${model_name}"
}

run_refresh_local_resources() {
	# Test a plain juju refresh with a local charm
	echo

	model_name="test-refresh-local-resources"
	file="${TEST_DIR}/${model_name}.log"
	charm_name="${TEST_DIR}/juju-qa-test.charm"

	ensure "${model_name}" "${file}"

	juju download juju-qa-test --no-progress - >"${charm_name}"
	# In 2.9 this charm is deploying with xenial. However there's
	# a bug in charm, opening the resource file throws:
	# TypeError: invalid file
	# The charm is using python 3. No error in ubuntu 20.04.
	juju deploy "${charm_name}" juju-qa-test --series focal --resource foo-file="./tests/suites/resources/foo-file.txt"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	juju refresh juju-qa-test --path "${charm_name}"

	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" "1")"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	juju config juju-qa-test foo-file=true
	# wait for config-changed, the charm will update the status
	# to include the contents of foo-file.txt
	wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"

	destroy_model "${model_name}"
}

run_refresh_channel() {
	# Test juju refresh from one channel to another
	echo

	model_name="test-refresh-channel"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy juju-qa-test
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	OUT=$(juju refresh juju-qa-test --channel 2.0/edge 2>&1 || true)
	# shellcheck disable=SC2059
	printf "${OUT}\n"

	# format: Added charm-store charm "ubuntu", revision 21 in channel stable, to the model
	revision=$(echo "${OUT}" | awk 'BEGIN{FS=","} {print $2}' | awk 'BEGIN{FS=" "} {print $2}')

	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" "${revision}")"
	wait_for "juju-qa-test" "$(charm_channel "juju-qa-test" "2.0/edge")"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	destroy_model "${model_name}"
}

run_refresh_channel_no_new_revision() {
	# Test juju refresh from one channel to another, with no new
	# charm revision.
	echo

	model_name="test-refresh-channel-no-new-revision"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy jameinel-ubuntu-lite
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"
	# get revision to ensure it doesn't change
	cs_revision=$(juju status --format json | jq -S '.applications | .["ubuntu-lite"] | .["charm-rev"]')

	juju refresh ubuntu-lite --channel edge

	wait_for "ubuntu-lite" "$(charm_channel "ubuntu-lite" "edge")"
	wait_for "ubuntu-lite" "$(charm_rev "ubuntu-lite" "${cs_revision}")"
	wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

	destroy_model "${model_name}"
}

test_basic() {
	if [ "$(skip 'test_basic')" ]; then
		echo "==> TEST SKIPPED: basic refresh"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_refresh_local"
		run "run_refresh_local_resources"
		run "run_refresh_channel"
		run "run_refresh_channel_no_new_revision"
	)
}
