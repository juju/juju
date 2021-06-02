test_spaces_manual() {
	if [ "$(skip 'test_spaces_manual')" ]; then
		echo "==> TEST SKIPPED: spaces manual"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"aws")
			export BOOTSTRAP_PROVIDER="manual"
			run "run_spaces_manual_aws"
			;;
		*)
			echo "==> TEST SKIPPED: deploy manual - tests for LXD and AWS"
			;;
		esac
	)
}

run_spaces_manual_aws() {
	echo

	echo "==> Checking for dependencies"
	check_dependencies aws

	name="tests-$(petname)"

	controller="${name}-controller"
	machine1="${name}-m1"
	machine2="${name}-m2"
	machine3="${name}-m3"

	set -eux

	add_clean_func "run_cleanup_deploy_manual_aws"

	# Eventually we should use BOOTSTRAP_SERIES.
	series="bionic"

	OUT=$(aws ec2 describe-images \
		--owners 099720109477 \
		--filters "Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-${series}-?????-amd64-server-????????" 'Name=state,Values=available' \
		--query 'reverse(sort_by(Images, &CreationDate))[:1].ImageId' \
		--output text)
	if [[ -z ${OUT} ]]; then
		echo "No image available: unknown state."
		exit 1
	fi
	image_id="${OUT}"

	# Get each subnet ID from the default VPC.
	# These should represent the VPC 172.31.0.0/16 carved into 3 /20 ranges.
	# See https://docs.aws.amazon.com/vpc/latest/userguide/default-vpc.html#default-vpc-components
	OUT=$(aws ec2 describe-subnets | jq -r '.Subnets[] | select(.DefaultForAz==true) | .SubnetId')
	len=$(echo "$OUT" | wc -w)
	if [[ ${len} -ne "3" ]]; then
		echo "Expected 3 subnets from default VPC; got: ${OUT}"
		exit 1
	fi
	subs="${OUT}"

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
	aws ec2 create-key-pair --key-name "${name}" --query 'KeyMaterial' --output text >~/.ssh/"${name}".pem
	chmod 400 ~/.ssh/"${name}".pem
	echo "${name}" >>"${TEST_DIR}/ec2-key-pairs"

	local addr_c addr_m1 addr_m2 addr_m3

	launch_and_wait_addr_ec2 "${name}" "${controller}" "${image_id}" "$(echo "${subs}" | sed -n 1p)" "${sg_id}" addr_c
	launch_and_wait_addr_ec2 "${name}" "${machine1}" "${image_id}" "$(echo "${subs}" | sed -n 1p)" "${sg_id}" addr_m1
	launch_and_wait_addr_ec2 "${name}" "${machine2}" "${image_id}" "$(echo "${subs}" | sed -n 2p)" "${sg_id}" addr_m2
	launch_and_wait_addr_ec2 "${name}" "${machine3}" "${image_id}" "$(echo "${subs}" | sed -n 3p)" "${sg_id}" addr_m3

	ensure_valid_ssh_hosts "${addr_c}" "${addr_m1}" "${addr_m2}" "${addr_m3}"

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

	juju add-cloud --client "${cloud_name}" "${TEST_DIR}/cloud_name.yaml" >"${TEST_DIR}/add-cloud.log" 2>&1

	file="${TEST_DIR}/test-${name}.log"

	bootstrap "${cloud_name}" "test-${name}" "${file}"

	# Each machine is in a different subnet,
	# which will be discovered when the agent starts.
	juju add-machine ssh:ubuntu@"${addr_m1}" >"${TEST_DIR}/add-machine-1.log" 2>&1
	juju add-machine ssh:ubuntu@"${addr_m2}" >"${TEST_DIR}/add-machine-2.log" 2>&1
	juju add-machine ssh:ubuntu@"${addr_m3}" >"${TEST_DIR}/add-machine-3.log" 2>&1

	# The discovered subnets then be carved into spaces.
	juju add-space space-1 172.31.0.0/20
	juju add-space space-2 172.31.16.0/20
	juju add-space space-3 172.31.32.0/20
}
