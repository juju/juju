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

    test_mongo_memory_profile
    test_enable_ha

    destroy_controller "test-controller"
}
