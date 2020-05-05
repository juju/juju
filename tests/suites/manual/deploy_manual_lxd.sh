create_priveleged_profile() {
    PROFILE=$(cat <<'EOF'
config:
  security.nesting: "true"
  security.privileged: "true"
description: create privileged container
devices: {}
name: profile-privileged
EOF
)
    echo "${PROFILE}" > "${TEST_DIR}/profile-privileged.yaml"

    OUT=$(lxc profile show profile-privileged || true)
    if [ -z "${OUT}" ]; then
        lxc profile create profile-privileged
        lxc profile edit profile-privileged < "${TEST_DIR}/profile-privileged.yaml"
    fi
}

create_user_profile() {
    local name

    name=${1}
    profile_name="profile-${name}"

    public_key="${TEST_DIR}/${name}.pub"
    key=$(cat "${public_key}" | tr -d '\n')

    PROFILE=$(cat <<EOF
config:
  user.user-data: |
    #cloud-config
    ssh_authorized_keys:
      - "${key}"
description: create user profile
devices: {}
name: ${profile_name}
EOF
)

    echo "${PROFILE}" > "${TEST_DIR}/${profile_name}.yaml"

    # Do not check if it already exists, we want to fail if it already exists.
    lxc profile create "${profile_name}"
    lxc profile edit "${profile_name}" < "${TEST_DIR}/${profile_name}.yaml"
}

delete_user_profile() {
    local name

    name=${1}
    profile_name="profile-${name}"

    set +e
    OUT=$(lxc profile show profile-privileged || true)
    if [ -n "${OUT}" ]; then
        lxc profile delete "${profile_name}"
    fi
    set_verbosity
}

run_deploy_manual_lxd() {
    echo

    echo "==> Checking for dependencies"
    check_dependencies lxd

    name="tests-$(petname)"

    ssh-keygen -f "${TEST_DIR}/${name}" \
        -t rsa \
        -C "ubuntu@${name}.com" \
        -N ""

    create_priveleged_profile
    create_user_profile "${name}"

    series="bionic"

    controller="${name}-controller"
    model1="${name}-m1"
    model2="${name}-m2"

    launch_and_wait_addr() {
        local container_name addr_result

        container_name=${1}
        addr_result=${2}

        lxc launch --profile default \
             --profile profile-privileged \
             --profile "profile-${name}" \
             ubuntu:"${series}" "${container_name}"

        local address=""

        attempt=0
        while [ ${attempt} -lt 30 ]; do
            address=$(lxc list "$1" --format json | \
                jq --raw-output '.[0].state.network.eth0.addresses | map(select( .family == "inet")) | .[0].address')

            if echo "${address}" | grep -q '^[0-9]\+\.[0-9]\+\.[0-9]\+\.[0-9]\+$'; then
                echo "Using container address ${address}"
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
EOF
)

    echo "${CLOUD}" > "${TEST_DIR}/cloud_name.yaml"

    manual_deploy "${cloud_name}" "${name}" "${addr_m1}" "${addr_m2}"
}