test_smoke() {
    if [ "$(skip 'test_smoke')" ]; then
        echo "==> TEST SKIPPED: smoke tests"
        return
    fi

    file="${TEST_DIR}/test-smoke.txt"

    bootstrap "test-smoke" "${file}"

    test_build
    test_deploy "${file}"

    destroy_controller "test-smoke"
}
