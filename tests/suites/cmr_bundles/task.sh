test_cmr_bundles() {
    if [ -n "${SKIP_CMR_BUNDLES:-}" ]; then
        echo "==> SKIP: Asked to skip CMR bundle tests"
        return
    fi

    file="${TEST_DIR}/test-cmr-bundles.txt"

    bootstrap "test-cmr-bundles" "${file}"

    test_deploy
    test_export_overlay

    destroy_controller "test-cmr-bundles"
}
