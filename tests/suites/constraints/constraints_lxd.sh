run_constraints_lxd() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/constraints-lxd.txt"

	ensure "constraints-lxd" "${file}"

	echo "Deploy 2 machines with different constraints"
	juju add-machine --constraints "cores=2"
	juju add-machine --constraints "cores=2 mem=2G"

	wait_for_machine_agent_status "0" "started"
	wait_for_machine_agent_status "1" "started"

	echo "Ensure machine 0 has 2 cores"
	machine0_hardware=$(juju machines --format json | yq -r '.["machines"]["0"]["hardware"]')
	check_contains "${machine0_hardware}" "cores=2"

	echo "Ensure machine 1 has 2 cores and 2G memory"
	machine1_hardware=$(juju machines --format json | yq -r '.["machines"]["1"]["hardware"]')
	check_contains "${machine1_hardware}" "cores=2"
	check_contains "${machine1_hardware}" "mem=2048M"

	echo "Deploy an application with model and application constraints"
	juju set-model-constraints "mem=1024M"
	juju deploy ubuntu-lite constraints-app --constraints "cores=2"

	machine_id=$(juju status --format yaml | yq -r '.applications."constraints-app".units."constraints-app/0".machine')
	wait_for_machine_agent_status "${machine_id}" "started"

	echo "Ensure the application stores only its constraints and resolved architecture"
	application_constraints=$(juju constraints constraints-app)
	check_contains "${application_constraints}" "arch="
	check_contains "${application_constraints}" "cores=2"
	check_not_contains "${application_constraints}" "mem="

	echo "Ensure the application machine receives the merged constraints"
	machine_constraints=$(juju show-machine "${machine_id}" --format json | yq -r ".machines[\"${machine_id}\"].constraints")
	check_contains "${machine_constraints}" "cores=2"
	check_contains "${machine_constraints}" "mem=1024M"

	echo "Update the application constraints"
	juju set-constraints constraints-app "cores=4 mem=2G"

	application_constraints=$(juju constraints constraints-app)
	check_contains "${application_constraints}" "cores=4"
	check_contains "${application_constraints}" "mem=2048M"
	check_not_contains "${application_constraints}" "arch="

	echo "Ensure a new unit still receives the merged constraints"
	juju add-unit constraints-app
	machine_id=$(juju status --format yaml | yq -r '.applications."constraints-app".units."constraints-app/1".machine')
	wait_for_machine_agent_status "${machine_id}" "started"

	machine_constraints=$(juju show-machine "${machine_id}" --format json | yq -r ".machines[\"${machine_id}\"].constraints")
	check_contains "${machine_constraints}" "cores=4"
	check_contains "${machine_constraints}" "mem=2048M"

	destroy_model "constraints-lxd"
}
