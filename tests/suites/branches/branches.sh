run_branch() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists
    file="${TEST_DIR}/test-branches.txt"
    ensure "branches" "${file}"

    juju branch | grep -q 'Active branch is "master"'

    juju deploy redis

    wait_for "redis" "$(idle_condition "redis")"

    juju add-branch test-branch
    juju branch | grep -q 'Active branch is "test-branch"'

    juju config redis password=pass --branch test-branch
    juju config redis password --branch test-branch | grep -q "pass"
    juju config redis password --branch master | wc -c | grep -q "0"

    juju commit test-branch | grep -q 'Active branch set to "master"'
    juju config redis password | grep -q "pass"

    # Clean up!
    destroy_model "branches"
}

test_branch() {
    if [ -n "$(skip 'test_branch')" ]; then
        echo "==> SKIP: Asked to skip branch tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_branch"
    )
}
