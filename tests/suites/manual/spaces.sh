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
			run "run_spaces_manual_aws"
			;;
		*)
			echo "==> TEST SKIPPED: spaces manual - tests AWS only"
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

	add_clean_func "run_cleanup_deploy_manual_aws"

	# Eventually we should use BOOTSTRAP_SERIES.
	series="jammy"

	echo "==> Configuring aws"
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

	echo "===> Ensure at least 3 default subnets exists"
	check_ge "$(aws ec2 describe-availability-zones | jq -r ".AvailabilityZones | length")" 3
	aws ec2 create-default-subnet --availability-zone "${BOOTSTRAP_REGION}a" 2>/dev/null || true
	aws ec2 create-default-subnet --availability-zone "${BOOTSTRAP_REGION}b" 2>/dev/null || true
	aws ec2 create-default-subnet --availability-zone "${BOOTSTRAP_REGION}c" 2>/dev/null || true

	sub1=$(aws ec2 describe-subnets | jq -r '.Subnets[] | select(.DefaultForAz==true and .CidrBlock=="172.31.0.0/20") | .SubnetId')
	sub2=$(aws ec2 describe-subnets | jq -r '.Subnets[] | select(.DefaultForAz==true and .CidrBlock=="172.31.16.0/20") | .SubnetId')
	sub3=$(aws ec2 describe-subnets | jq -r '.Subnets[] | select(.DefaultForAz==true and .CidrBlock=="172.31.32.0/20") | .SubnetId')

	# Ensure we have a security group allowing SSH and controller access.
	OUT=$(aws ec2 describe-security-groups | jq '.SecurityGroups[] | select(.GroupName=="ci-spaces-manual-ssh")' || true)
	if [[ -z ${OUT} ]]; then
		sg_id=$(aws ec2 create-security-group --group-name "ci-spaces-manual-ssh" --description "SSH access for manual spaces test" --query 'GroupId' --output text)
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 22 --cidr 0.0.0.0/0
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 17070 --cidr 0.0.0.0/0
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol udp --port 17070 --cidr 0.0.0.0/0
		# 37017 is required for mongo and aws with public ip to init replica set.
		# See findSelfInConfig in src/mongo/db/repl/repl_set_config_checks.cpp and isSelf in src/mongo/db/repl/isself.cpp
		# isSelfFastPath: checks if a host:port matches a local interface mongo is bound to.
		# isSelfSlowPath: checks if a host:port can be dialed and reaches the current mongo daemon.
		# Since elastic IPs are not bound to a local interface (instead handled by AWS through routing rules)
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 37017 --cidr 0.0.0.0/0
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 0-65535 --source-group "${sg_id}"
		aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol udp --port 0-65535 --source-group "${sg_id}"
	else
		sg_id=$(echo "${OUT}" | jq -r '.GroupId')
	fi

	# Create a key-pair so that we can provision machines via SSH.
	aws ec2 create-key-pair --key-name "${name}" --query 'KeyMaterial' --output text >"${TEST_DIR}/${name}.pem"
	chmod 400 "${TEST_DIR}/${name}.pem"
	echo "${name}" >>"${TEST_DIR}/ec2-key-pairs"

	local addr_c addr_m1 addr_m2 addr_m3

	echo "===> Creating machines in aws"
	launch_and_wait_addr_ec2 "${name}" "${controller}" "${image_id}" "${sub1}" "${sg_id}" addr_c
	launch_and_wait_addr_ec2 "${name}" "${machine1}" "${image_id}" "${sub1}" "${sg_id}" addr_m1
	launch_and_wait_addr_ec2 "${name}" "${machine2}" "${image_id}" "${sub2}" "${sg_id}" addr_m2
	launch_and_wait_addr_ec2 "${name}" "${machine3}" "${image_id}" "${sub3}" "${sg_id}" addr_m3

	ensure_valid_ssh_config "${name}.pem" "${addr_c}" "${addr_m1}" "${addr_m2}" "${addr_m3}"

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

	juju add-cloud --client "${cloud_name}" "${TEST_DIR}/cloud_name.yaml" 2>&1 | tee "${TEST_DIR}/add-cloud.log"

	file="${TEST_DIR}/test-${name}.log"

	export BOOTSTRAP_PROVIDER="manual"
	unset BOOTSTRAP_REGION
	bootstrap "${cloud_name}" "test-${name}" "${file}"

	# Each machine is in a different subnet,
	# which will be discovered when the agent starts.
	echo "==> Adding Machines"
	juju add-machine ssh:ubuntu@"${addr_m1}" 2>&1 | tee "${TEST_DIR}/add-machine-1.log"
	juju add-machine ssh:ubuntu@"${addr_m2}" 2>&1 | tee "${TEST_DIR}/add-machine-2.log"
	juju add-machine ssh:ubuntu@"${addr_m3}" 2>&1 | tee "${TEST_DIR}/add-machine-3.log"

	wait_for_machine_agent_status "0" "started"
	wait_for_machine_agent_status "1" "started"
	wait_for_machine_agent_status "2" "started"

	# The discovered subnets then be carved into spaces.
	juju add-space space-1 172.31.0.0/20
	juju add-space space-2 172.31.16.0/20
	juju add-space space-3 172.31.32.0/20
}
