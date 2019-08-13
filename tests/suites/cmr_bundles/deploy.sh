run_deploy() {
    echo

    file="${TEST_DIR}/cmr_bundles_test_deploy.txt"

    bootstrap lxd "cmr_bundles_test_deploy" "${file}"
}

test_deploy() {
    if [ -n "${SKIP_CMR_BUNDLES_DEPLOY:-}" ]; then
        echo "==> SKIP: Asked to skip CMR bundle deploy tests"
        return
    fi

    (
        set -e

        run "deploy"
    )
}
