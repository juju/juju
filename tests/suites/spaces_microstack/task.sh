test_spaces_microstack() {
    if [ "$(skip 'test_spaces_microstack')" ]; then
        echo "==> TEST SKIPPED: space tests (Microstack)"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju multipass

    vm_name=$(rnd_name "vm")
    vm_file="${TEST_DIR}/test-spaces-microstack_vm.txt"

    launch_vm "${vm_name}" "${vm_file}" -c 8 -m 12G
    mount_juju "${vm_name}"
    setup_microstack "${vm_name}"

    desc="Test"
    file="${TEST_DIR}/exec-test-microstack.txt"

    set +e
    vm_exec "${vm_name}" "${desc}" "${file}" cd "/home/ubuntu/go/src/github.com/juju/juju/tests" && \
        ./main.sh spaces_microstack test_spaces_microstack_nested
    set_verbosity

    # Get the results from the test!
}

mount_juju() {
    local name

    name=${1}

    OUT=$(which juju | grep -E "^${GOPATH}" || true)
    if [ $? -ne 0 ]; then
        echo "expected juju bin to be in GOPATH"
        exit 1
    fi

    mount_vm_dir "${name}" "${GOPATH}" "/home/ubuntu/go"
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
