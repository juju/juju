run_resource_upgrade() {
    echo
    name="resource-upgrade"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    juju deploy juju-qa-test --channel 2.0/edge
    wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"
    juju config juju-qa-test foo-file=true

    # wait for update-status
    wait_for "resource line one: testing one plus one." "$(workload_status juju-qa-test 0).message"
    juju config juju-qa-test foo-file=false

    juju refresh juju-qa-test --channel 2.0/stable
    wait_for "juju-qa-test" "$(charm_channel "juju-qa-test" "2.0/stable")"

    juju config juju-qa-test foo-file=true
    wait_for "resource line one: testing one." "$(workload_status juju-qa-test 0).message"

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

    juju config juju-qa-test foo-file=true
    # wait for config-changed, the charm will update the status
    # to include the contents of foo-file.txt
    wait_for "resource line one: did the resource attach?" "$(workload_status juju-qa-test 0).message"

    destroy_model "test-${name}"
}

run_resource_attach_large() {
    echo
    name="resource-attach-large"

    file="${TEST_DIR}/test-${name}.log"

    ensure "test-${name}" "${file}"

    juju deploy juju-qa-test
    wait_for "juju-qa-test" "$(idle_condition "juju-qa-test")"

    # .txt suffix required for attach.
    FILE=$(mktemp /tmp/resource-XXXXX.txt)
    # Use urandom to add alpha numeric characters with new lines added to the file
    cat /dev/urandom | base64 | head -c 100M > "${FILE}"
    line=$(head -n 1 "${FILE}")
    juju attach juju-qa-test foo-file="${FILE}"

    juju config juju-qa-test foo-file=true
    # wait for config-changed, the charm will update the status
    # to include the contents of foo-file.txt
    wait_for "resource line one: ${line}" "$(workload_status juju-qa-test 0).message"

    rm "${FILE}"
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

        run "run_resource_upgrade"
        run "run_resource_attach"
        run "run_resource_attach_large"
    )
}