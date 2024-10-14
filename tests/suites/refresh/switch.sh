# Copyright 2024 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

run_refresh_switch_local_to_ch_channel() {
	# Test juju refresh from a local charm to a charm hub charm with a specific channel
	echo

	model_name="test-refresh-local-switch-ch"
	file="${TEST_DIR}/${model_name}.log"
	charm_name="${TEST_DIR}/ubuntu.charm"

	ensure "${model_name}" "${file}"

	juju download juju-qa-refresher --no-progress - >"${charm_name}"
	juju deploy --channel=stable "${charm_name}"
	wait_for "refresher" "$(idle_condition "refresher")"

	OUT=$(juju refresh refresher --switch ch:juju-qa-refresher --channel edge 2>&1 || true)
	if echo "${OUT}" | grep -E -vq "Added charm-hub charm"; then
		# shellcheck disable=SC2046
		echo $(red "failed refreshing charm: ${OUT}")
		exit 5
	fi
	# shellcheck disable=SC2059
	printf "${OUT}\n"

	# format: Added charm-store charm "juju-qa-refresher", revision 1 in channel stable, to the model
	revision=$(echo "${OUT}" | awk 'BEGIN{FS=","} {print $2}' | awk 'BEGIN{FS=" "} {print $2}')

	wait_for "refresher" "$(charm_rev "refresher" "${revision}")"
	wait_for "refresher" "$(charm_channel "refresher" "latest/edge")"
	wait_for "refresher" "$(idle_condition "refresher")"

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

		run "run_refresh_switch_local_to_ch_channel"
	)
}
