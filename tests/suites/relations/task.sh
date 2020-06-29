test_relations() {
    if [ "$(skip 'test_relations')" ]; then
        echo "==> TEST SKIPPED: relation tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-relations.log"

    bootstrap "test-relations" "${file}"

    test_relation_data_exchange
    test_relation_departing_unit
    test_relation_list_app

    destroy_controller "test-relations"
}
