test_hooktools() {
    if [ "$(skip 'test_hooktools')" ]; then
        echo "==> TEST SKIPPED: hook tools"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-hooktools.txt"

    bootstrap "test-hooktools" "${file}"

    test_state_hook_tools

    destroy_controller "test-hooktools"
}