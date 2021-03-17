test_controller() {
    if [ "$(skip 'test_controller')" ]; then
        echo "==> TEST SKIPPED: controller tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-controller.log"

    bootstrap "test-controller" "${file}"

    test_enable_ha
    # Leave this one last, as it can cause mongo to slowdown to a snails pace.
    test_mongo_memory_profile

    destroy_controller "test-controller"
}
