run_controller_limit_access_in_ha() {
	local controller_unit controller_machine_id machine_info instance_id region_or_az network_tag_or_group

	controller_unit=${1}
	controller_machine_id=$(juju status -m controller --format=json |
		yq -r ".applications.controller.units[\"${controller_unit}\"].machine")
	machine_info="$(juju show-machine -m controller "${controller_machine_id}" --format=json)"
	instance_id="$(yq -r ".machines[\"${controller_machine_id}\"].\"instance-id\"" <<<"${machine_info}")"
	region_or_az=$(region_or_availability_zone "${controller_machine_id}")
	network_tag_or_group=$(instance_network_tag_or_group "${controller_machine_id}")

	echo "Limit access to all controllers in HA"
	juju expose -m controller controller --to-cidrs 10.0.0.0/24
	# In HA the firewaller restricts port 17070 on each controller
	# machine sequentially via separate provider firewall calls, so
	# juju status keeps succeeding until all machines are restricted.
	wait_for_or_fail "! timeout 5 juju status"

	echo "Temporarily grant this machine access to the first controller in HA"
	allow_access_to_api_port "${instance_id}" "${region_or_az}" "${network_tag_or_group}"
	wait_for_or_fail "timeout 5 juju status"

	echo "Allow access to all controllers in HA from anywhere"
	juju expose -m controller controller --to-cidrs 0.0.0.0/0

	# Restore access before removing the temporary firewall rule so subsequent
	# tests can use the controller normally.
	remove_access_to_api_port "${instance_id}" "${region_or_az}" "${network_tag_or_group}"
	wait_for_or_fail "timeout 5 juju status"
}

run_limit_access_ha() {
	local file base_unit unit
	local -a controller_units

	echo
	file="${TEST_DIR}/limit_access_ha.log"
	ensure "limit-access-ha" "${file}"

	wait_for_controller_unit_count 1
	mapfile -t controller_units < <(controller_unit_names)
	if [[ ${#controller_units[@]} -ne 1 ]]; then
		echo "expected one controller unit before HA limit-access test, found: ${controller_units[*]}"
		exit 1
	fi
	base_unit=${controller_units[0]}
	wait_for_ha 1

	juju add-unit -m controller controller -n 2
	wait_for_controller_unit_count 3
	mapfile -t controller_units < <(controller_unit_names)
	if ! printf '%s\n' "${controller_units[@]}" | grep -Fxq "${base_unit}"; then
		echo "controller unit ${base_unit} disappeared during scale out"
		exit 1
	fi
	wait_for_ha 3
	run_controller_limit_access_in_ha "${base_unit}"

	# Return to one unit, retaining the newest ordinal for the next test.
	while [[ ${#controller_units[@]} -gt 1 ]]; do
		unit=${controller_units[0]}
		remove_controller_unit "${unit}"
		wait_for_controller_unit_count $((${#controller_units[@]} - 1))
		mapfile -t controller_units < <(controller_unit_names)
		wait_for_ha "${#controller_units[@]}"
	done

	destroy_model "limit-access-ha"
}

test_limit_access_ha() {
	if [ -n "$(skip 'test_limit_access_ha')" ]; then
		echo "==> SKIP: Asked to skip controller HA limit-access test"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"ec2" | "gce")
			run "run_limit_access_ha"
			;;
		*)
			echo "==> TEST SKIPPED: HA limit-access test runs on aws/gce only"
			;;
		esac
	)
}
