test_model() {
    if [ "$(skip 'test_model')" ]; then
        echo "==> TEST SKIPPED: model tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-models.txt"

    export JUJU_DEV_FEATURE_FLAGS=cmr-migrations

    bootstrap "test-models" "${file}"

    # Test that need to be run are added here!
    test_model_migration

    destroy_controller "test-models"
}
