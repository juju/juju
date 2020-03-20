test_spaces_microstack_nested() {
    if [ "$(skip 'test_spaces_microstack_nested')" ]; then
        echo "==> TEST SKIPPED: space tests (Microstack)"
        return
    fi

    set_verbosity

    export PATH=${HOME}/go/bin:$PATH
    export BOOTSTRAP_PROVIDER="microstack"

    echo "==> Checking for dependencies"
    check_dependencies juju "microstack.openstack"

    export OS_SERIES=bionic
    export OS_REGION=microstack

    setup_image_metadata
    setup_simplestreams
    setup_cloud
    setup_credentials

    file="${TEST_DIR}/test-spaces-microstack.txt"

    bootstrap "test-spaces-microstack" "${file}" --bootstrap-series=${OS_SERIES} \
        --metadata-source="${TEST_DIR}"/simplestreams \
        --model-default network=test \
        --model-default external-network=external \
        --model-default use-floating-ip=true

    test_add_space

    destroy_controller "test-spaces-microstack"
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
    file="${TEST_DIR}/juju-metadata.txt"

    mkdir -p "${TEST_DIR}/simplestreams"
    KEYSTONE_IP=10.20.20.1
    juju metadata generate-image \
        -d "${TEST_DIR}"/simplestreams \
        -i "${IMAGE_ID}" \
        -s "${OS_SERIES}" \
        -r "${OS_REGION}" \
        -u "http://${KEYSTONE_IP}:5000/v3" >"${file}" 2>&1
}

setup_cloud() {
    OUT=$(juju clouds --client --format=json 2>/dev/null | jq -r "select(.microstack)")
    if [ -n "${OUT}" ]; then
        OUT=$(juju remove-credential --client microstack admin >/dev/null 2>&1 || true)
        OUT=$(juju remove-cloud --client microstack >/dev/null 2>&1 || true)
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
    juju add-cloud --client microstack -f "${TEST_DIR}"/microstack.yaml 2>/dev/null
}

setup_credentials() {
    OUT=$(juju credentials --format json | jq -r ".[\"client-credentials\"] | select(.microstack)")
    if [ -n "${OUT}" ]; then
        set +e
        OUT=$(juju remove-credential --client microstack admin >/dev/null 2>&1 || true)
        set_verbosity
    fi

    ENV_FILE="/var/snap/microstack/common/etc/microstack.rc"
    if [ ! -f "${ENV_FILE}" ]; then
        echo "Expected microstack file to exist."
        exit 1
    fi

    # shellcheck disable=SC1090
    . "${ENV_FILE}"

    CREDS=$(cat <<EOF
credentials:
    microstack:
        ${OS_USERNAME}:
            auth-type: userpass
            username: ${OS_USERNAME}
            password: ${OS_PASSWORD}
            project-domain-name: ${OS_PROJECT_DOMAIN_NAME}
            user-domain-name: ${OS_USER_DOMAIN_NAME}
            domain-name: ""
            tenant-id: ""
            tenant-name: ${OS_USERNAME}
            version: "3"
EOF
)
    echo "${CREDS}" > "${TEST_DIR}"/creds.yaml
    juju add-credential microstack --client -f "${TEST_DIR}"/creds.yaml 2>/dev/null
}