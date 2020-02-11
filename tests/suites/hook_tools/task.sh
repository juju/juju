test_hook_tools() {
      if [ "$(skip 'test_hook_tools')" ]; then
        echo "==> TEST SKIPPED: hook tools"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-hook-tools.txt"

    bootstrap "test-hook-tools" "${file}"

    test_state_hook_tools

    destroy_controller "test-hook-tools"
}