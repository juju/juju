multipass_version() {
    version=$(multipass version | grep -oP "multipass\s+\K([0-9a-z\+\.\-]+)")
    echo "${version}"
}

ensure_vm() {
    local name

    name=${1}
    shift

    OUT=$(multipass list --format=json | jq -r ".list | .[] | .name" | grep "${name}" || true)
    if [ "${OUT}" != "${name}" ]; then
        launch_vm "${name}" "$@"
    fi
}

launch_vm() {
    local name output

    name=${1}
    shift

    output=${1}
    shift

    if [ -z "${name}" ]; then
        rnd=$(head /dev/urandom | tr -dc a-z0-9 | head -c 8; echo '')
        name="vm-${rnd}"
    fi

    if [ ! -f "${TEST_DIR}/multipasses" ]; then
        touch "${TEST_DIR}/multipasses"
    fi

    version=$(multipass_version)

    START_TIME=$(date +%s)

    echo "====> Launching Multipass VM ($(green "${version}"))"
    multipass_launch "${name}" "${output}" "$@"

    END_TIME=$(date +%s)
    echo "====> Launched Multipass VM ($((END_TIME-START_TIME))s)"

    export VM_NAME="${name}"
}

mount_vm_dir() {
    local name src dest

    name=${1}
    src=${2}
    dest=${3}

    echo "====> Mounting Folder VM ($(green "${name}"))"
    multipass mount "${src}" "${name}":"${dest}"
    echo "====> Mounted Folder VM"
}

vm_exec() {
    local name output

    name=${1}
    shift

    desc=${1}
    shift

    output=${1}
    shift

    START_TIME=$(date +%s)

    echo "====> Start exec VM ($(green "${name}":"${desc}"))"
    multipass exec -v "${name}" -- "$@" > "${output}" 2>&1

    END_TIME=$(date +%s)
    echo "====> Finished exec VM ($((END_TIME-START_TIME))s)"
}

multipass_launch() {
    local name output

    name=${1}
    shift

    output=${1}
    shift

    verbose=""
    if [ "${VERBOSE}" -gt 1 ]; then
        verbose="-v"
    fi

    if [ -z "${output}" ]; then
        multipass launch "${verbose}" -n "${name}" "$@" > "${output}" 2>&1
    else
        multipass launch "${verbose}" -n "${name}" "$@"
    fi
    
    echo "${name}" >> "${TEST_DIR}/multipasses"
}

destroy_vm() {
    local name

    name=${1}
    shift

    echo "====> Destroying vm ${name}"
    OUT=$(multipass list --format=json | jq -r ".list | .[] | .name" | grep "${name}" || true)
    if [ ! -z "${OUT}" ]; then
        output="${TEST_DIR}/${name}-vm-destroy.txt"

        set +e
        multipass delete -p "${name}"
        set_verbosity

        sed -i "/^${name}$/d" "${TEST_DIR}/multipasses"
    fi
    echo "====> Destroyed vm ${name}"
}

cleanup_multipasses() {
    if ! which multipass >/dev/null 2>&1; then
        echo "====> Multipass skipped"
        exit 0
    fi

    if [ -f "${TEST_DIR}/multipasses" ]; then
        echo "====> Cleaning up multipasses"

        while read -r vm_name; do
            destroy_vm "${vm_name}"
        done < "${TEST_DIR}/multipasses"
        rm -f "${TEST_DIR}/multipasses" || true
    fi
    echo "====> Completed cleaning up multipasses"
}