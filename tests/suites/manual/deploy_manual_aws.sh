run_deploy_manual_aws() {
    echo

    echo "==> Checking for dependencies"
    check_dependencies aws

    name="tests-$(petname)"
    series="bionic"

    controller="${name}-controller"
    model1="${name}-m1"
    model2="${name}-m2"

    set -eux

    add_clean_func "run_cleanup_deploy_manual_aws"

    vpc_id=$(aws ec2 create-vpc --cidr-block 10.0.0.0/28 --query 'Vpc.VpcId' --output text)
    aws ec2 modify-vpc-attribute --vpc-id "${vpc_id}" --enable-dns-support "{\"Value\":true}"
    aws ec2 modify-vpc-attribute --vpc-id "${vpc_id}" --enable-dns-hostnames "{\"Value\":true}"

    echo "${vpc_id}" >> "${TEST_DIR}/ec2-vpcs"

    igw_id=$(aws ec2 create-internet-gateway --query 'InternetGateway.InternetGatewayId' --output text)
    aws ec2 attach-internet-gateway --internet-gateway-id "${igw_id}" --vpc-id "${vpc_id}"

    subnet_id=$(aws ec2 create-subnet --vpc-id "${vpc_id}" --cidr-block 10.0.0.0/28 --query 'Subnet.SubnetId' --output text)
    routetable_id=$(aws ec2 create-route-table --vpc-id "${vpc_id}" --query 'RouteTable.RouteTableId' --output text)

    aws ec2 associate-route-table --route-table-id "${routetable_id}" --subnet-id "${subnet_id}"
    aws ec2 create-route --route-table-id "${routetable_id}" --destination-cidr-block 0.0.0.0/0 --gateway-id "${igw_id}"

    sg_id=$(aws ec2 create-security-group --group-name "ci-manual-deploy" --description "run_deploy_manual_aws" --vpc-id "${vpc_id}" --query 'GroupId' --output text)
    aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 22 --cidr 0.0.0.0/0
    aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 0-65535 --cidr 0.0.0.0/0
    aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol udp --port 0-65535 --cidr 0.0.0.0/0
    aws ec2 authorize-security-group-egress --group-id "${sg_id}" --protocol tcp --port 0-65535 --cidr 0.0.0.0/0
    aws ec2 authorize-security-group-egress --group-id "${sg_id}" --protocol udp --port 0-65535 --cidr 0.0.0.0/0

    aws ec2 create-key-pair --key-name "${name}" --query 'KeyMaterial' --output text > ~/.ssh/"${name}".pem
    chmod 400 ~/.ssh/"${name}".pem

    launch_and_wait_addr() {
        local name instance_name addr_result

        name=${1}
        instance_name=${2}
        addr_result=${3}

        tags="ResourceType=instance,Tags=[{Key=Name,Value=${instance_name}}]"
        instance_id=$(aws ec2 run-instances --image-id ami-03d8261f577d71b6a \
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
        ssh-keygen -f "${HOME}/.ssh/known_hosts" -R "${addr}"

        attempt=0
        while [ ${attempt} -lt 10 ]; do
            OUT=$(ssh -T -n -i ~/.ssh/"${name}".pem \
                -o IdentitiesOnly=yes \
                -o StrictHostKeyChecking=no \
                -o AddKeysToAgent=yes \
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

    if [ -f "${TEST_DIR}/ec2-vpcs" ]; then
        echo "====> Cleaning up ec2-vpcs"
        while read -r vpc_id; do
            delete_vpc "${vpc_id}" >>"${TEST_DIR}/aws_cleanup"
        done < "${TEST_DIR}/ec2-vpcs"
    fi

    set_verbosity

    echo "====> Completed cleaning up aws"
}

delete_vpc() {
    local vpc_id

    vpc_id=${1}

    echo "====> Cleaning up ec2-vpc (${vpc_id})"

    aws ec2 describe-internet-gateways --filter Name=attachment.vpc-id,Values="${vpc_id}" | jq -r '.InternetGateways[].InternetGatewayId' | 
      while read igw_id; do
          aws ec2 detach-internet-gateway --internet-gateway-id="${igw_id}" --vpc-id="${vpc_id}" >>"${TEST_DIR}/aws_cleanup"
          aws ec2 delete-internet-gateway --internet-gateway-id="${igw_id}" >>"${TEST_DIR}/aws_cleanup"
      done
    
    aws ec2 describe-subnets --filters Name=vpc-id,Values=${vpc_id} | jq -r '.Subnets[].SubnetId' |
      while read subnet_id; do
          aws ec2 delete-subnet --subnet-id "${subnet_id}" >>"${TEST_DIR}/aws_cleanup"
      done

    aws ec2 describe-security-groups --filter Name=vpc-id,Values="${vpc_id}" | jq -r '.SecurityGroups[] | select(.GroupName != "default") | .GroupId' | 
      while read sg_id; do
          aws ec2 delete-security-group --group-id "${sg_id}" >>"${TEST_DIR}/aws_cleanup"
      done

    aws ec2 describe-route-tables --filter Name=vpc-id,Values="${vpc_id}" | jq -r '.RouteTables[].RouteTableId' |
      while read route_id; do
          aws ec2 delete-route-table --route-table-id "${route_id}" >>"${TEST_DIR}/aws_cleanup"
      done

    aws ec2 delete-vpc --vpc-id ${vpc_id} >>"${TEST_DIR}/aws_cleanup"
}