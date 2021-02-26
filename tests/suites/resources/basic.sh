
run_deploy_repo_resource(){
    echo
    name="deploy-repo-resource"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    juju deploy juju-qa-test --channel 2.0/stable
    wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
    juju config juju-qa-test file-foo=true

    # wait for update-status
    wait_for "resource line one: testing stable?" "$(workload_status juju-qa-test 0).message"

    destroy_model "test-${name}"
}

run_deploy_local_resource(){
    echo
    name="deploy-local-resource"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    juju deploy juju-qa-test --resource foo-file="${TEST_DIR}/foo-file.txt"
    wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
    juju config juju-qa-test file-foo=true

    # wait for update-status
    wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"

    destroy_model "test-${name}"
}

test_basic_resources() {
    if [ "$(skip 'test_basic_resources')" ]; then
        echo "==> TEST SKIPPED: Resource basics"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_deploy_local_resource"
        run "run_deploy_repo_resource"
    )
}