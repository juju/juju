launch_and_wait_ec2() {
	local instance_name instance_id_result instance_type

	instance_name=${1}
	instance_image_id=${2}
	subnet_id=${3}
	instance_id_result=${4}

	arch="amd64"
	if [[ -n ${MODEL_ARCH} ]]; then
		arch="${MODEL_ARCH}"
	fi
	case "${arch}" in
	"amd64")
		instance_type="t3.medium"
		;;
	"arm64")
		instance_type="m6g.large"
		;;
	*)
		echo "Unrecognised arch ${arch}"
		exit
		;;
	esac

	tags="ResourceType=instance,Tags=[{Key=Name,Value=${instance_name}}]"
	instance_id=$(aws ec2 run-instances --image-id "${instance_image_id}" \
		--count 1 \
		--instance-type ${instance_type} \
		--associate-public-ip-address \
		--tag-specifications "${tags}" \
		--subnet-id "${subnet_id}" \
		--query 'Instances[0].InstanceId' \
		--output text)

	echo "${instance_id}" >>"${TEST_DIR}/ec2-instances"

	aws ec2 wait instance-running --instance-ids "${instance_id}"

	# shellcheck disable=SC2086
	eval $instance_id_result="'${instance_id}'"
}

create_ami_and_wait_available() {
	local ami_id_result
	ami_id_result=${1}

	arch="amd64"
	if [[ -n ${MODEL_ARCH} ]]; then
		arch="${MODEL_ARCH}"
	fi

	# Retrieve the image_id corresponding to ubuntu jammy
	OUT=$(aws ec2 describe-images \
		--owners 099720109477 \
		--filters "Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-jammy-?????-${arch}-server-????????" 'Name=state,Values=available' \
		--query 'reverse(sort_by(Images, &CreationDate))[:1].ImageId' \
		--output text)
	if [[ -z ${OUT} ]]; then
		echo "No image available: unknown state."
		exit 1
	fi
	image_id="${OUT}"

	# Retrieve the subnet id
	sub1=$(aws ec2 describe-subnets | jq -r '.Subnets[] | select(.DefaultForAz==true and .CidrBlock=="172.31.0.0/20") | .SubnetId')

	# Launch an ec2 instance using the retrieved jammy image id
	local instance_id_ami_builder
	launch_and_wait_ec2 "constraints_aws_ami_builder" "${image_id}" "${sub1}" "instance_id_ami_builder"
	echo "Created instance for ami builder: ${instance_id_ami_builder}"

	# Create the ami from the ec2 instance_id
	ami_id=$(aws ec2 create-image --instance-id "${instance_id_ami_builder}" --name "test_ami_constraints" --description "Test ami used for constraints tests" --output text)

	aws ec2 wait image-available --image-ids "${ami_id}"
	echo "${ami_id}" >>"${TEST_DIR}/ec2-amis"
	echo "Created ami: ${ami_id}"

	# shellcheck disable=SC2086
	eval $ami_id_result="'${ami_id}'"
}

run_cleanup_ami() {
	set +e

	if [[ -f "${TEST_DIR}/ec2-instances" ]]; then
		echo "====> Cleaning up EC2 instances"
		while read -r ec2_instance; do
			aws ec2 terminate-instances --instance-ids="${ec2_instance}" >>"${TEST_DIR}/aws_cleanup"
		done <"${TEST_DIR}/ec2-instances"
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
