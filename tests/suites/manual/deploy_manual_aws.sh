run_deploy_manual_aws() {
    echo

    echo "==> Checking for dependencies"
    check_dependencies aws

    name="tests-$(petname)"

    ssh-keygen -f "${TEST_DIR}/${name}" \
        -t rsa \
        -C "ubuntu@${name}.com" \
        -N ""
    
    series="bionic"

    controller="${name}-controller"
    model1="${name}-m1"
    model2="${name}-m2"

    launch_and_wait_addr() {
        local instance_name addr_result

        instance_name=${1}
        addr_result=${2}

        tags="ResourceType=instance,Tags=[{Key=Name,Value=${instance_name}}]"
        reservation_id=$(aws ec2 run-instances --image-id ami-03d8261f577d71b6a \
            --count 1 \
            --instance-type t2.medium \
            --associate-public-ip-address \
            --tag-specifications "${tags}" | \
            jq -r ".ReservationId")

        local address=""

        attempt=0
        while [ ${attempt} -lt 30 ]; do
            address=$(aws ec2 describe-instances \
                --query "Reservations[?ReservationId=='${reservation_id}'].Instances[*].{Instance:PublicIpAddress}" | \
                jq -r ".[] | .[] | .Instance" || true)

            if echo "${address}" | grep -q '^[0-9]\+\.[0-9]\+\.[0-9]\+\.[0-9]\+$'; then
                echo "Using instance address ${address}"
                break
            fi
            sleep 1
            attempt=$((attempt+1))
        done

        # shellcheck disable=SC2086
        eval $addr_result="'${address}'"
    }

    launch_and_wait_addr "${controller}" addr_c
    launch_and_wait_addr "${model1}" addr_m1
    launch_and_wait_addr "${model2}" addr_m2

    # shellcheck disable=SC2154
    for addr in "${addr_c}" "${addr_m1}" "${addr_m2}"; do
        ssh-keygen -f "${HOME}/.ssh/known_hosts" -R "${addr}"

        attempt=0
        while [ ${attempt} -lt 10 ]; do
            OUT=$(ssh -T -n -i "${TEST_DIR}/${name}" \
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