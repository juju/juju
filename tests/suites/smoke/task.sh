test_smoke() {
    if [ -n "${SKIP_SMOKE:-}" ]; then
        echo "==> SKIP: Asked to skip smoke tests"
        return
    fi

    file="${TEST_DIR}/test-smoke.txt"

    bootstrap "test-smoke" "${file}"

    test_build
    test_deploy

    destroy_controller "test-smoke"
}
