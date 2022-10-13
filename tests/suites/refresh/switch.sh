run_refresh_switch_cs_to_ch() {
	echo

	model_name="test-refresh-switch-ch"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy cs:ubuntu-19
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	OUT=$(juju refresh ubuntu --switch ch:ubuntu 2>&1 || true)
	if echo "${OUT}" | grep -E -vq "Added charm-hub charm"; then
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	printf "${OUT}\n"

	# Added local charm "ubuntu", revision 2, to the model
	revision=$(echo "${OUT}" | awk 'BEGIN{FS=","} {print $2}' | awk 'BEGIN{FS=" "} {print $2}')

	wait_for "ubuntu" "$(charm_rev "ubuntu" "${revision}")"
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	destroy_model "${model_name}"
}

run_refresh_switch_cs_to_ch_channel() {
	echo

	model_name="test-refresh-switch-ch-channel"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy cs:ubuntu-19
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	OUT=$(juju refresh ubuntu --switch ch:ubuntu --channel edge 2>&1 || true)
	if echo "${OUT}" | grep -E -vq "in channel edge"; then
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	printf "${OUT}\n"

	# Added local charm "ubuntu", revision 2, to the model
	revision=$(echo "${OUT}" | awk 'BEGIN{FS=","} {print $2}' | awk 'BEGIN{FS=" "} {print $2}')

	wait_for "ubuntu" "$(charm_rev "ubuntu" "${revision}")"
	wait_for "ubuntu" "$(charm_channel "ubuntu" "edge")"
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	destroy_model "${model_name}"
}

run_refresh_switch_local_to_ch_channel() {
	echo

	model_name="test-refresh-local-switch-ch"
	file="${TEST_DIR}/${model_name}.log"
	charm_name="${TEST_DIR}/ubuntu.charm"

	ensure "${model_name}" "${file}"

	juju download ubuntu --no-progress - >"${charm_name}"
	juju deploy "${charm_name}" ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	OUT=$(juju refresh ubuntu --switch ch:ubuntu --channel edge 2>&1 || true)
	if echo "${OUT}" | grep -E -vq "Added charm-hub charm"; then
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	printf "${OUT}\n"

	# Added local charm "ubuntu", revision 2, to the model
	revision=$(echo "${OUT}" | awk 'BEGIN{FS=","} {print $2}' | awk 'BEGIN{FS=" "} {print $2}')

	wait_for "ubuntu" "$(charm_rev "ubuntu" "${revision}")"
	wait_for "ubuntu" "$(charm_channel "ubuntu" "edge")"
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	destroy_model "${model_name}"
}

run_refresh_switch_channel() {
	echo

	model_name="test-refresh-switch-channel"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	juju deploy juju-qa-test
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	OUT=$(juju refresh juju-qa-test --channel 2.0/edge 2>&1 || true)
	printf "${OUT}\n"

	# Added local charm "ubuntu", revision 2, to the model
	revision=$(echo "${OUT}" | awk 'BEGIN{FS=","} {print $2}' | awk 'BEGIN{FS=" "} {print $2}')

	wait_for "juju-qa-test" "$(charm_rev "juju-qa-test" "${revision}")"
	wait_for "juju-qa-test" "$(charm_channel "juju-qa-test" "2.0/edge")"
	wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

	destroy_model "${model_name}"
}

test_switch() {
	if [ "$(skip 'test_switch')" ]; then
		echo "==> TEST SKIPPED: refresh switch"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_refresh_switch_cs_to_ch"
		run "run_refresh_switch_cs_to_ch_channel"
		run "run_refresh_switch_local_to_ch_channel"
		run "run_refresh_switch_channel"
	)
}
