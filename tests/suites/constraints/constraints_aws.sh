run_constraints_aws() {
	name="constraints-aws"

	# Echo out to ensure nice output to the test suite.
	echo
	echo "==> Checking for dependencies"
	check_dependencies aws

	file="${TEST_DIR}/constraints-aws.txt"

	ensure "${name}" "${file}"

	# In order to test the image-id constraint in AWS, we need to create a ami.
	add_clean_func "run_cleanup_ami"
	local ami_id
	create_ami_and_wait_available "ami_id"

	echo "Deploy 2 machines with different constraints"
	juju add-machine --constraints "cores=2"
	juju add-machine --constraints "image-id=${ami_id}"

	wait_for_machine_agent_status "0" "started"
	wait_for_machine_agent_status "1" "started"

	echo "Ensure machine 0 has 2 cores"
	machine0_hardware=$(juju machines --format json | jq -r '.["machines"]["0"]["hardware"]')
	check_contains "${machine0_hardware}" "cores=2"

	echo "Ensure machine 1 uses the correct AMI ID from image-id constraint"
	machine_instance_id=$(juju show-machine --format json | jq -r '.["machines"]["1"]["instance-id"]')
	aws ec2 describe-instances --instance-ids "${machine_instance_id}" --query 'Reservations[0].Instances[0].ImageId' --output text | check "${ami_id}"

	destroy_model "${name}"
}
