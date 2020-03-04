run_branch() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists
    file="${TEST_DIR}/test-branches.txt"
    ensure "branches" "${file}"

    juju branch | check 'Active branch is "master"'

    juju deploy redis

    wait_for "redis" "$(idle_condition "redis")"

    juju add-branch test-branch
    juju branch | check 'Active branch is "test-branch"'

    juju config redis password=pass --branch test-branch
    juju config redis password --branch test-branch | check "pass"
    juju config redis password --branch master | wc -c | check "0"

    juju commit test-branch | check 'Active branch set to "master"'
    juju config redis password | check "pass"

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
