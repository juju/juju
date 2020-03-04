run_example1() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists
    file="${TEST_DIR}/test-example1.txt"
    ensure "example1" "${file}"

    # Run your checks here
    echo "Hello example 1!" | check "Hello example 1!"

    # Clean up!
    destroy_model "example1"
}

run_example2() {
    echo

    file="${TEST_DIR}/test-example2.txt"
    ensure "example2" "${file}"

    echo "Hello example 2!" | check "Hello example 2!"

    destroy_model "example2"
}

test_example() {
    if [ -n "$(skip 'test_example')" ]; then
        echo "==> SKIP: Asked to skip example tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_example1"
        run "run_example2"
    )
}
