test_relations() {
    if [ "$(skip 'test_relations')" ]; then
        echo "==> TEST SKIPPED: relation tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-relations.txt"

    bootstrap "test-relations" "${file}"

    test_relation_data_exchange

    destroy_controller "test-relations"
}
