test_spaces_microstack() {
    if [ "$(skip 'test_spaces_microstack')" ]; then
        echo "==> TEST SKIPPED: space tests (Microstack)"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju petname multipass

    vm_name=$(petname)
    vm_file="${TEST_DIR}/test-spaces-microstack_vm.txt"

    launch_vm "${vm_name}" "${vm_file}" -c 8 -m 12G
    mount_juju "${vm_name}"
    setup_microstack "${vm_name}"

    exit

    export OS_SERIES=bionic
    export OS_REGION=microstack

    setup_image_metadata
    setup_simplestreams
    setup_cloud
    setup_credentials

    file="${TEST_DIR}/test-spaces-microstack.txt"

    bootstrap "test-spaces-microstack" "${file}" --bootstrap-series=$OS_SERIES \
        --metadata-source="${TEST_DIR}"/simplestreams \
        --model-default network=test \
        --model-default external-network=external \
        --model-default use-floating-ip=true

    test_add_space

    destroy_controller "test-spaces-microstack"
}

mount_juju() {
    local name

    name=${1}

    mount_vm_dir "${name}" $(dirname $PWD) "/home/ubuntu/go/github.com/juju/juju"
}

setup_microstack() {
    local name

    name=${1}

    desc="Install Microstack"
    file="${TEST_DIR}/exec-install-microstack.txt"
    vm_exec "${name}" "${desc}" "${file}" sudo snap install microstack --edge --devmode

    desc="Init Microstack"
    file="${TEST_DIR}/exec-init-microstack.txt"
    vm_exec "${name}" "${desc}" "${file}" sudo microstack.init --auto
}

setup_image_metadata() {
    OUT=$(microstack.openstack image list -f json | jq -r ".[] | select(.Name == \"${OS_SERIES}\") | .ID" || true)
    if [ -z "${OUT}" ]; then
        IMAGE_ID=$(curl "http://cloud-images.ubuntu.com/${OS_SERIES}/current/${OS_SERIES}-server-cloudimg-amd64.img" | \
            microstack.openstack image create \
            --public --container-format=bare --disk-format=qcow2 \
            -f value -c id "${OS_SERIES}")
    else
        IMAGE_ID="${OUT}"
    fi
    export IMAGE_ID
}

setup_simplestreams() {
    mkdir -p "${TEST_DIR}/simplestreams"
    KEYSTRONE_IP=10.20.20.1
    juju metadata generate-image \
        -d ~/simplestreams \
        -i $IMAGE_ID \
        -s $OS_SERIES \
        -r $OS_REGION \
        -u http://$KEYSTONE_IP:5000/v3
}

setup_cloud() {
    OUT=$(juju clouds --client --format=json 2>/dev/null | jq -r "select(.microstack)")
    if [ -n "${OUT}" ]; then
        echo "Microstack cloud in juju already exists."
        exit 0
    fi

    CLOUD=$(cat <<'EOF'
clouds:
    microstack:
      type: openstack
      auth-types: [access-key,userpass]
      regions:
        microstack:
           endpoint: http://10.20.20.1:5000/v3
EOF
    )
    echo "${CLOUD}" > "${TEST_DIR}"/microstack.yaml
    juju add-cloud microstack --client -f "${TEST_DIR}"/microstack.yaml
}

setup_credentials() {
    OUT=$(juju credentials --format json | jq -r ".[\"client-credentials\"] | select(.microstack)")
    if [ -n "${OUT}" ]; then
        echo "Microstack credentials in juju already exist."
        exit 0
    fi

    ENV_FILE="/var/snap/microstack/common/etc/microstack.rc"
    if [ ! -f "${ENV_FILE}" ]; then
        echo "Expected microstack file to exist."
        exit 1
    fi

    source "${ENV_FILE}"

    CREDS=$(cat <<EOF
credentials:
    microstack:
        ${OS_USERNAME}:
            auth-type: userpass
            username: ${OS_USERNAME}
            password: ${OS_PASSWORD}
            project-domain-name: ${OS_PROJECT_DOMAIN_NAME}
            user-domain-name: ${OS_USER_DOMAIN_NAME}
EOF
)
    echo "${CREDS}" > "${TEST_DIR}"/creds.yaml
    juju add-credential microstack --client -f "${TEST_DIR}"/creds.yaml
}