run_constraints_aws() {
	name="constraints-aws"

	# Echo out to ensure nice output to the test suite.
	echo
	echo "==> Checking for dependencies"
	check_dependencies aws

	file="${TEST_DIR}/constraints-aws.txt"

	ensure "${name}" "${file}"

	add_clean_func "run_cleanup_constraints_aws"

	# In order to test the image-id constraint in AWS, we need to create a
	# ami, but for that we first need to launch an ec2 instance from which
	# the ami will be created.
	#
	# Retrieve the image_id corresponding to ubuntu jammy
	OUT=$(aws ec2 describe-images \
		--owners 099720109477 \
		--filters "Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-jammy-?????-amd64-server-????????" 'Name=state,Values=available' \
		--query 'reverse(sort_by(Images, &CreationDate))[:1].ImageId' \
		--output text)
	if [[ -z ${OUT} ]]; then
		echo "No image available: unknown state."
		exit 1
	fi
	image_id="${OUT}"

	# Retrieve the subnet id
	sub1=$(aws ec2 describe-subnets | jq -r '.Subnets[] | select(.DefaultForAz==true and .CidrBlock=="172.31.0.0/20") | .SubnetId')

	# Ensure we have a security group allowing SSH and controller access.
	OUT=$(aws ec2 describe-security-groups | jq '.SecurityGroups[] | select(.GroupName=="ci-spaces-manual-ssh")' || true)
	if [[ -z ${OUT} ]]; then
		sg_id=$(aws ec2 create-security-group --group-name "ci-spaces-manual-ssh" --description "SSH access for manual spaces test" --query 'GroupId' --output text)
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 22 --cidr 0.0.0.0/0
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 17070 --cidr 0.0.0.0/0
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol udp --port 17070 --cidr 0.0.0.0/0
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 0-65535 --source-group "${sg_id}"
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol udp --port 0-65535 --source-group "${sg_id}"
	else
		sg_id=$(echo "${OUT}" | jq -r '.GroupId')
	fi

	# Create a key-pair so that we can provision machines via SSH.
	aws ec2 create-key-pair --key-name "${name}" --query 'KeyMaterial' --output text >"${TEST_DIR}/${name}.pem"
	chmod 400 "${TEST_DIR}/${name}.pem"
	echo "${name}" >>"${TEST_DIR}/ec2-key-pairs"

	# Launch an ec2 instance using the retrieved jammy image id
	local instance_id_ami_builder
	launch_and_wait_ec2 "${name}" "constraints_aws_ami_builder" "${image_id}" "${sub1}" "${sg_id}" instance_id_ami_builder
	echo "Created instance for ami builder: ${instance_id_ami_builder}"

	# Create the ami from the ec2 instace_id
	local ami_id
	create_ami_and_wait_available "${instance_id_ami_builder}" ami_id

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
	aws ec2 describe-instances --instance-ids ${machine_instance_id} --query 'Reservations[0].Instances[0].ImageId' --output text | check "${ami_id}"

	destroy_model "${name}"
}
