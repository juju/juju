run_model_config() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists.
    file="${TEST_DIR}/test-model-config.log"
    ensure "model-config" "${file}"

    juju model-config mode=strict
    juju model-config mode | grep "strict"
    juju model-config mode=""
    juju model-config mode | wc -m | grep "0"
    juju model-config mode="boom" || echo "ERROR" | grep "ERROR"


    destroy_model "model-config"
}

test_model_config() {
    if [ -n "$(skip 'test_model_config')" ]; then
        echo "==> SKIP: Asked to skip model config tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_model_config"
    )
}