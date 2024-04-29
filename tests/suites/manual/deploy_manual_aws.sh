create_vpc() {
	set_verbosity
	vpc_id=$(aws ec2 create-vpc --cidr-block 10.0.0.0/28 --query 'Vpc.VpcId' --tag-specifications 'ResourceType=vpc,Tags=[{Key=Name,Value=manual-deploy}]' --output text)
	aws ec2 wait vpc-available --vpc-ids "${vpc_id}"
	aws ec2 modify-vpc-attribute --vpc-id "${vpc_id}" --enable-dns-support '{"Value":true}' >/dev/null
	aws ec2 modify-vpc-attribute --vpc-id "${vpc_id}" --enable-dns-hostnames '{"Value":true}' >/dev/null

	echo $vpc_id
}

create_igw() {
	set_verbosity
	igw_id=$(aws ec2 create-internet-gateway --query 'InternetGateway.InternetGatewayId' --output text)
	aws ec2 attach-internet-gateway --internet-gateway-id "${igw_id}" --vpc-id "${vpc_id}" >/dev/null

	echo $igw_id
}

create_subnet() {
	set_verbosity
	subnet_id=$(aws ec2 create-subnet --vpc-id "${vpc_id}" --cidr-block 10.0.0.0/28 --query 'Subnet.SubnetId' --output text)

	routetable_id=$(aws ec2 create-route-table --vpc-id "${vpc_id}" --query 'RouteTable.RouteTableId' --output text)

	aws ec2 associate-route-table --route-table-id "${routetable_id}" --subnet-id "${subnet_id}" >/dev/null
	aws ec2 create-route --route-table-id "${routetable_id}" --destination-cidr-block 0.0.0.0/0 --gateway-id "${igw_id}" >/dev/null

	echo $subnet_id
}

create_secgroup() {
	set_verbosity
	sg_id=$(aws ec2 create-security-group --group-name "ci-manual-deploy" --description "run_deploy_manual_aws" --vpc-id "${vpc_id}" --query 'GroupId' --output text)
	aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 22 --cidr 0.0.0.0/0 >/dev/null
	aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 0-65535 --cidr 0.0.0.0/0 >/dev/null
	aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol udp --port 0-65535 --cidr 0.0.0.0/0 >/dev/null
	aws ec2 authorize-security-group-egress --group-id "${sg_id}" --protocol tcp --port 0-65535 --cidr 0.0.0.0/0 >/dev/null
	aws ec2 authorize-security-group-egress --group-id "${sg_id}" --protocol udp --port 0-65535 --cidr 0.0.0.0/0 >/dev/null

	echo $sg_id
}

run_deploy_manual_aws() {
	echo

	echo "==> Checking for dependencies"
	check_dependencies aws

	name="tests-$(petname)"

	controller="${name}-controller"
	model1="${name}-m1"
	model2="${name}-m2"

	add_clean_func "run_cleanup_deploy_manual_aws"

	# Eventually we should use BOOTSTRAP_BASE.
	series="jammy"

	# This creates a new VPC for this deployment. If one already exists it will
	# get the existing setup and use that.
	# The ingress and egress for this setup is rather lax, but in time we can
	# tighten that up.
	# All instances and key-pairs should be cleaned up on exiting.
	OUT=$(aws ec2 describe-images \
		--owners 099720109477 \
		--filters "Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-${series}-?????-amd64-server-????????" 'Name=state,Values=available' \
		--query 'reverse(sort_by(Images, &CreationDate))[:1].ImageId' \
		--output text)
	if [[ -z ${OUT} ]]; then
		echo "No image available: unknown state."
		exit 1
	fi
	instance_image_id="${OUT}"

	local vpc_id igw_id sg_id subnet_id

	OUT=$(aws ec2 describe-vpcs | jq '.Vpcs[] | select(.Tags[]? | select((.Key=="Name") and (.Value=="manual-deploy")))' || true)
	if [[ -z ${OUT} ]]; then
		vpc_id=$(create_vpc)
		echo "===> Created vpc $vpc_id"
	else
		vpc_id=$(echo "${OUT}" | jq -r '.VpcId' || true)
		echo "===> Re-using vpc $vpc_id"
	fi

	OUT=$(aws ec2 describe-internet-gateways | jq ".InternetGateways[] | select(.Attachments[0].VpcId == \"${vpc_id}\")")
	if [[ -z ${OUT} ]]; then
		igw_id=$(create_igw)
		echo "===> Created igw $igw_id"
	else
		igw_id=$(echo "${OUT}" | jq -r '.InternetGatewayId')
		echo "===> Re-using igw $igw_id"
	fi

	OUT=$(aws ec2 describe-subnets | jq ".Subnets[] | select(.VpcId == \"${vpc_id}\")" || true)
	if [[ -z ${OUT} ]]; then
		subnet_id=$(create_subnet)
		echo "===> Created subnet $subnet_id"
	else
		subnet_id=$(echo "${OUT}" | jq -r '.SubnetId')
		echo "===> Re-using subnet $subnet_id"
	fi

	OUT=$(aws ec2 describe-security-groups | jq ".SecurityGroups[] | select(.VpcId==\"${vpc_id}\" and .GroupName==\"ci-manual-deploy\")" || true)
	if [[ -z ${OUT} ]]; then
		sg_id=$(create_secgroup)
		echo "===> Created secgroup $sg_id"
	else
		sg_id=$(echo "${OUT}" | jq -r '.GroupId')
		echo "===> Re-using secgroup $sg_id"
	fi

	aws ec2 create-key-pair --key-name "${name}" --query 'KeyMaterial' --output text >"${TEST_DIR}/${name}.pem"
	chmod 400 "${TEST_DIR}/${name}.pem"
	echo "${name}" >>"${TEST_DIR}/ec2-key-pairs"

	local addr_c addr_m1 addr_m2

	echo "===> Creating machines in aws"
	launch_and_wait_addr_ec2 "${name}" "${controller}" "${instance_image_id}" "${subnet_id}" "${sg_id}" addr_c
	launch_and_wait_addr_ec2 "${name}" "${model1}" "${instance_image_id}" "${subnet_id}" "${sg_id}" addr_m1
	launch_and_wait_addr_ec2 "${name}" "${model2}" "${instance_image_id}" "${subnet_id}" "${sg_id}" addr_m2

	ensure_valid_ssh_config "${name}.pem" "${addr_c}" "${addr_m1}" "${addr_m2}"

	cloud_name="cloud-${name}"

	CLOUD=$(
		cat <<EOF
clouds:
  ${cloud_name}:
    type: manual
    endpoint: "ubuntu@${addr_c}"
    regions:
      default:
        endpoint: "ubuntu@${addr_c}"
EOF
	)

	echo "${CLOUD}" >"${TEST_DIR}/cloud_name.yaml"

	manual_deploy "${cloud_name}" "${name}" "${addr_m1}" "${addr_m2}"
}
