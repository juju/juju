
run_deploy_repo_resource(){
    echo
    name="deploy-repo-resource"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    juju deploy juju-qa-test --channel candidate
    wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
    juju config juju-qa-test foo-file=true

    # wait for update-status
    wait_for "resource line one: testing four." "$(workload_status juju-qa-test 0).message"

    destroy_model "test-${name}"
}

run_deploy_local_resource(){
    echo
    name="deploy-local-resource"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    juju deploy juju-qa-test --resource foo-file="./tests/suites/resources/foo-file.txt"
    wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
    juju config juju-qa-test foo-file=true

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