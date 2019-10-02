test_examples() {
    if [ "$(skip 'test_example')" ]; then
        echo "==> TEST SKIPPED: example tests"
        return
    fi

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-example.txt"

    bootstrap "test-example" "${file}"

    # Test that need to be run are added here!
    test_example
    test_other

    destroy_controller "test-example"
}
