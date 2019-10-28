run_deploy_charm() {
    echo

    file="${TEST_DIR}/test-deploy.txt"

    ensure "test-deploy" "${file}"

    juju deploy cs:~jameinel/ubuntu-lite-7
    wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

    destroy_model "test-cmr-bundles-deploy"
}

test_deploy_charms() {
    if [ "$(skip 'test_deploy_charms')" ]; then
        echo "==> TEST SKIPPED: deploy charms"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_deploy_charm"
    )
}
