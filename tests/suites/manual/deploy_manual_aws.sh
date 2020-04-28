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
    # All instances should be cleaned up on exiting.
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

    launch_and_wait_addr() {
        local name instance_name addr_result

        name=${1}
        instance_name=${2}
        addr_result=${3}

        tags="ResourceType=instance,Tags=[{Key=Name,Value=${instance_name}}]"
        instance_id=$(aws ec2 run-instances --image-id "${instance_image_id}" \
            --count 1 \
            --instance-type t2.medium \
            --associate-public-ip-address \
            --tag-specifications "${tags}" \
            --key-name "${name}" \
            --security-group-ids "${sg_id}" \
            --subnet-id "${subnet_id}" \
            --query 'Instances[0].InstanceId' \
            --output text)

        echo "${instance_id}" >> "${TEST_DIR}/ec2-instances"

        aws ec2 wait instance-running --instance-ids "${instance_id}"
        sleep 10

        address=$(aws ec2 describe-instances --instance-ids "${instance_id}" --query 'Reservations[0].Instances[0].PublicDnsName' --output text)

        # shellcheck disable=SC2086
        eval $addr_result="'${address}'"
    }

    launch_and_wait_addr "${name}" "${controller}" addr_c
    launch_and_wait_addr "${name}" "${model1}" addr_m1
    launch_and_wait_addr "${name}" "${model2}" addr_m2

    # shellcheck disable=SC2154
    for addr in "${addr_c}" "${addr_m1}" "${addr_m2}"; do
        ssh-keygen -f "${HOME}/.ssh/known_hosts" -R ubuntu@"${addr}"

        attempt=0
        while [ ${attempt} -lt 10 ]; do
            OUT=$(ssh -T -n -i ~/.ssh/"${name}".pem \
                -o IdentitiesOnly=yes \
                -o StrictHostKeyChecking=no \
                -o AddKeysToAgent=yes \
                -o UserKnownHostsFile="${HOME}/.ssh/known_hosts" \
                ubuntu@"${addr}" 2>&1 || true)
            if echo "${OUT}" | grep -q -v "Could not resolve hostname"; then
                echo "Adding ssh key to ${addr}"
                break
            fi

            sleep 1
            attempt=$((attempt+1))
        done

        if [ "${attempt}" -ge 10 ]; then
            echo "Failed to add key to ${addr}"
            exit 1
        fi
    done


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

run_cleanup_deploy_manual_aws() {
    set +e

    if [ -f "${TEST_DIR}/ec2-instances" ]; then
        echo "====> Cleaning up ec2-instances"
        while read -r ec2_instance; do
            aws ec2 terminate-instances --instance-ids="${ec2_instance}" >>"${TEST_DIR}/aws_cleanup"
        done < "${TEST_DIR}/ec2-instances"
    fi

    set_verbosity

    echo "====> Completed cleaning up aws"
}
