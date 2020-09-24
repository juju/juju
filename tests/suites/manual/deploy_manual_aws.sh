run_deploy_manual_aws() {
    echo

    echo "==> Checking for dependencies"
    check_dependencies aws

    name="tests-$(petname)"

    controller="${name}-controller"
    model1="${name}-m1"
    model2="${name}-m2"

    set -eux

    add_clean_func "run_cleanup_deploy_manual_aws"

    # Eventually we should use BOOTSTRAP_SERIES
    series="bionic"

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
    if [ -z "${OUT}" ]; then
        echo "No image available: unknown state."
        exit 1
    fi
    instance_image_id="${OUT}"

    local vpc_id sg_id subnet_id

    OUT=$(aws ec2 describe-vpcs | jq '.Vpcs[] | select(.Tags[]? | select((.Key=="Name") and (.Value=="manual-deploy")))' || true)
    vpc_id=$(echo "${OUT}" | jq -r '.VpcId' || true)
    if [ -z "${vpc_id}" ]; then
        # VPC doesn't exist, create one along with all the required setup.
        vpc_id=$(aws ec2 create-vpc --cidr-block 10.0.0.0/28 --query 'Vpc.VpcId' --output text)
        aws ec2 wait vpc-available --vpc-ids "${vpc_id}"
        aws ec2 create-tags --resources "${vpc_id}" --tags Key=Name,Value="manual-deploy"

        aws ec2 modify-vpc-attribute --vpc-id "${vpc_id}" --enable-dns-support "{\"Value\":true}"
        aws ec2 modify-vpc-attribute --vpc-id "${vpc_id}" --enable-dns-hostnames "{\"Value\":true}"

        igw_id=$(aws ec2 create-internet-gateway --query 'InternetGateway.InternetGatewayId' --output text)
        aws ec2 attach-internet-gateway --internet-gateway-id "${igw_id}" --vpc-id "${vpc_id}"

        subnet_id=$(aws ec2 create-subnet --vpc-id "${vpc_id}" --cidr-block 10.0.0.0/28 --query 'Subnet.SubnetId' --output text)
        aws ec2 create-tags --resources "${subnet_id}" --tags Key=Name,Value="manual-deploy"

        routetable_id=$(aws ec2 create-route-table --vpc-id "${vpc_id}" --query 'RouteTable.RouteTableId' --output text)

        aws ec2 associate-route-table --route-table-id "${routetable_id}" --subnet-id "${subnet_id}"
        aws ec2 create-route --route-table-id "${routetable_id}" --destination-cidr-block 0.0.0.0/0 --gateway-id "${igw_id}"

        sg_id=$(aws ec2 create-security-group --group-name "ci-manual-deploy" --description "run_deploy_manual_aws" --vpc-id "${vpc_id}" --query 'GroupId' --output text)
        aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 22 --cidr 0.0.0.0/0
        aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 0-65535 --cidr 0.0.0.0/0
        aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol udp --port 0-65535 --cidr 0.0.0.0/0
        aws ec2 authorize-security-group-egress --group-id "${sg_id}" --protocol tcp --port 0-65535 --cidr 0.0.0.0/0
        aws ec2 authorize-security-group-egress --group-id "${sg_id}" --protocol udp --port 0-65535 --cidr 0.0.0.0/0
    else
        OUT=$(aws ec2 describe-subnets | jq '.Subnets[] | select(.Tags[]? | select((.Key=="Name") and (.Value=="manual-deploy")))' || true)
        if [ -z "${OUT}" ]; then
            echo "Subnet not found: unknown state."
            echo "Delete VPC and start again."
            exit 1
        fi
        subnet_id=$(echo "${OUT}" | jq -r '.SubnetId')

        OUT=$(aws ec2 describe-security-groups | jq ".SecurityGroups[] | select(.VpcId==\"${vpc_id}\" and .GroupName==\"ci-manual-deploy\")" || true)
        if [ -z "${OUT}" ]; then
            echo "Security group not found: unknown state."
            echo "Delete VPC and start again."
            exit 1
        fi
        sg_id=$(echo "${OUT}" | jq -r '.GroupId')
    fi

    aws ec2 create-key-pair --key-name "${name}" --query 'KeyMaterial' --output text > ~/.ssh/"${name}".pem
    chmod 400 ~/.ssh/"${name}".pem
    echo "${name}" >> "${TEST_DIR}/ec2-key-pairs"

    launch_and_wait_addr_ec2 "${name}" "${controller}" "${instance_image_id}" "${subnet_id}" "${sg_id}" addr_c
    launch_and_wait_addr_ec2 "${name}" "${model1}" "${instance_image_id}" "${subnet_id}" "${sg_id}" addr_m1
    launch_and_wait_addr_ec2 "${name}" "${model2}" "${instance_image_id}" "${subnet_id}" "${sg_id}" addr_m2

    ensure_valid_ssh_hosts "${addr_c}" "${addr_m1}" "${addr_m2}"

    cloud_name="cloud-${name}"

    CLOUD=$(cat <<EOF
clouds:
  ${cloud_name}:
    type: manual
    endpoint: "ubuntu@${addr_c}"
    regions:
      default:
        endpoint: "ubuntu@${addr_c}"
EOF
)

    echo "${CLOUD}" > "${TEST_DIR}/cloud_name.yaml"

    manual_deploy "${cloud_name}" "${name}" "${addr_m1}" "${addr_m2}"
}

