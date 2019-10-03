test_cmr_bundles() {
    if [ "$(skip 'test_cmr_bundles')" ]; then
        echo "==> TEST SKIPPED: CMR bundle tests"
        return
    fi

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-cmr-bundles.txt"

    bootstrap "test-cmr-bundles" "${file}"

    test_deploy
    test_export_overlay

    destroy_controller "test-cmr-bundles"
}
