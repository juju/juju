run_resource_upgrade() {
    echo
    name="resource-upgrade"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    # Add test details

    destroy_model "test-${name}"
}

run_resource_attach() {
    echo
    name="resource-attach"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    juju deploy juju-qa-test
    wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
    juju attach juju-qa-test foo-file="./tests/suites/resources/foo-file.txt"

    sleep 5

    juju config juju-qa-test foo-file=true
    # wait for config-changed, the charm will update the status
    # to include the contents of foo-file.txt
    wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"

    destroy_model "test-${name}"
}

test_upgrade_resources() {
    if [ "$(skip 'test_upgrade_resources')" ]; then
        echo "==> TEST SKIPPED: Resource upgrades"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        #run "run_resource_upgrade"
        run "run_resource_attach"
    )
}