run_deploy() {
    echo

    file="${TEST_DIR}/test-cmr-bundles-deploy.txt"

    ensure "cmr-bundles-test-deploy" "${file}"

    juju deploy mysql
    wait_for "mysql" ".applications | keys[0]"

    juju offer mysql:db
    juju add-model other

    juju switch other

    bundle=./tests/suites/cmr_bundles/bundles/cmr_bundles_test_deploy.yaml
    sed "s/{{BOOTSTRAPPED_JUJU_CTRL_NAME}}/${BOOTSTRAPPED_JUJU_CTRL_NAME}/g" "${bundle}" > "${TEST_DIR}/cmr_bundles_test_deploy.yaml"
    juju deploy "${TEST_DIR}/cmr_bundles_test_deploy.yaml"

    destroy_model "cmr-bundles-test-deploy"
}

test_deploy() {
    if [ "$(skip 'test_deploy')" ]; then
        echo "==> TEST SKIPPED: CMR bundle deploy tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_deploy"
    )
}
