launch_and_wait_ec2() {
	local name instance_name instance_id_result

	name=${1}
	instance_name=${2}
	instance_image_id=${3}
	subnet_id=${4}
	sg_id=${5}
	instance_id_result=${6}

	tags="ResourceType=instance,Tags=[{Key=Name,Value=${instance_name}}]"
	instance_id=$(aws ec2 run-instances --image-id "${instance_image_id}" \
		--count 1 \
		--instance-type t2.medium \
		--associate-public-ip-address \
		--tag-specifications "${tags}" \
		--key-name "${name}" \
		--subnet-id "${subnet_id}" \
		--security-group-ids "${sg_id}" \
		--query 'Instances[0].InstanceId' \
		--output text)

	echo "${instance_id}" >>"${TEST_DIR}/ec2-instances"

	aws ec2 wait instance-running --instance-ids "${instance_id}"

	# shellcheck disable=SC2086
	eval $instance_id_result="'${instance_id}'"
}

create_ami_and_wait_available() {
	local instance_id_ami_builder ami_id_result

	instance_id_ami_builder=${1}
	ami_id_result=${2}

	ami_id=$(aws ec2 create-image --instance-id "${instance_id_ami_builder}" --name "test_ami_constraints" --description "Test ami used for constraints tests" --output text)

	aws ec2 wait image-available --image-ids "${ami_id}"
	echo "${ami_id}" >>"${TEST_DIR}/ec2-amis"
	echo "Created ami: ${ami_id}"

	# shellcheck disable=SC2086
	eval $ami_id_result="'${ami_id}'"
}

run_cleanup_constraints_aws() {
	set +e

	if [[ -f "${TEST_DIR}/ec2-instances" ]]; then
		echo "====> Cleaning up EC2 instances"
		while read -r ec2_instance; do
			aws ec2 terminate-instances --instance-ids="${ec2_instance}" >>"${TEST_DIR}/aws_cleanup"
		done <"${TEST_DIR}/ec2-instances"
	fi

	if [[ -f "${TEST_DIR}/ec2-key-pairs" ]]; then
		echo "====> Cleaning up EC2 key-pairs"
		while read -r ec2_keypair; do
			aws ec2 delete-key-pair --key-name="${ec2_keypair}" >>"${TEST_DIR}/aws_cleanup"
		done <"${TEST_DIR}/ec2-key-pairs"
	fi

	if [[ -f "${TEST_DIR}/ec2-amis" ]]; then
		echo "====> Cleaning up EC2 AMIs"
		while read -r ec2_ami; do
			aws ec2 deregister-image --image-id="${ec2_ami}" >>"${TEST_DIR}/aws_cleanup"
		done <"${TEST_DIR}/ec2-amis"
	fi

	set_verbosity

	echo "====> Completed cleaning up aws"
}
