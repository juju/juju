run_deploy() {
    echo

    file="${TEST_DIR}/cmr_bundles_test_deploy.txt"

    bootstrap lxd "cmr_bundles_test_deploy" "${file}"

    juju deploy mysql
    wait_for "mysql" ".applications | keys[0]"

    juju offer mysql:db
    juju add-model other

    juju switch other
    juju deploy ./tests/suites/cmr_bundles/bundles/cmr_bundles_test_deploy.yaml

    destroy "cmr_bundles_test_deploy"
}

test_deploy() {
    if [ -n "${SKIP_CMR_BUNDLES_DEPLOY:-}" ]; then
        echo "==> SKIP: Asked to skip CMR bundle deploy tests"
        return
    fi

    (
        set -e

        cd ../

        run "deploy"
    )
}
