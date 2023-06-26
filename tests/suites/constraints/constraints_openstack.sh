run_constraints_openstack() {
	name="constraints-openstack"

	# Echo out to ensure nice output to the test suite.
	echo
	check_dependencies openstack

	file="${TEST_DIR}/constraints-openstack.txt"

	ensure "${name}" "${file}"

	# The openstack cluster must contain an image named 'jammy', otherwise
	# the test cannot run.
	echo "Ensure there is an image with name 'jammy'"
	jammy_id=$(openstack image list -f json --name jammy | jq -r '.[] | .ID')
	if [[ -z ${jammy_id} ]]; then
		echo "No image available with name 'jammy' on openstack"
		exit 1
	fi

	juju add-machine --constraints "image-id=${jammy_id}"

	wait_for_machine_agent_status "0" "started"

	echo "Ensure machine 0 uses the correct image ID from image-id constraint"
	juju_machine_name=$(juju show-machine --format json | jq -r '.["machines"]["0"]["hostname"]')
	openstack server list -f json --name ${juju_machine_name} | jq -r '.[] | .Image' | check "jammy"

	destroy_model "${name}"
}
