run_deploy_bundle() {
    echo

    file="${TEST_DIR}/test-bundles-deploy.txt"

    ensure "test-bundles-deploy" "${file}"

    juju deploy cs:~juju-qa/bundle/basic-0
    wait_for "ubuntu" ".applications | keys[0]"
    wait_for "ubuntu-lite" ".applications | keys[1]"

    destroy_model "test-bundles-deploy"
}

run_deploy_cmr_bundle() {
    echo

    file="${TEST_DIR}/test-cmr-bundles-deploy.txt"

    ensure "test-cmr-bundles-deploy" "${file}"

    juju deploy mysql
    wait_for "mysql" ".applications | keys[0]"

    juju offer mysql:db
    juju add-model other

    juju switch other

    bundle=./tests/suites/deploy/bundles/cmr_bundles_test_deploy.yaml
    sed "s/{{BOOTSTRAPPED_JUJU_CTRL_NAME}}/${BOOTSTRAPPED_JUJU_CTRL_NAME}/g" "${bundle}" > "${TEST_DIR}/cmr_bundles_test_deploy.yaml"
    juju deploy "${TEST_DIR}/cmr_bundles_test_deploy.yaml"

    destroy_model "test-cmr-bundles-deploy"
}

test_deploy_bundles() {
    if [ "$(skip 'test_deploy_bundles')" ]; then
        echo "==> TEST SKIPPED: deploy bundles"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_deploy_bundle"
        run "run_deploy_cmr_bundle"
    )
}
