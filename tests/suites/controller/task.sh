test_controller() {
    if [ "$(skip 'test_controller')" ]; then
        echo "==> TEST SKIPPED: controller tests"
        return
    fi

    file="${TEST_DIR}/test-controller.txt"

    bootstrap "test-controller" "${file}"

    test_mongo_memory_profile

    destroy_controller "test-controller"
}
