run_model_migration() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists
    file="${TEST_DIR}/test-model-migration.txt"
    ensure "model-migration" "${file}"

    # Ensure we have another controller available
    bootstrap_alt_controller "alt-model-migration"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju deploy ./tests/suites/model/charms/ubuntu

    wait_for "ubuntu" "$(idle_condition "ubuntu")"

    juju migrate "model-migration" "alt-model-migration"
    juju switch "alt-model-migration"

    # Wait for the new model migration to appear in the alt controller.
    wait_for_model "model-migration"

    # Once the model has appeared, switch to it.
    juju switch "alt-model-migration:model-migration"

    wait_for "ubuntu" "$(idle_condition "ubuntu")"

    # Clean up!
    destroy_controller "alt-model-migration"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    destroy_model "model-migration"
}

test_model_migration() {
    if [ -n "$(skip 'test_model_migration')" ]; then
        echo "==> SKIP: Asked to skip model tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_model_migration"
    )
}

bootstrap_alt_controller() {
    local name

    name=${1}

    START_TIME=$(date +%s)
    echo "====> Bootstrapping destination juju"

    file="${TEST_DIR}/${name}.txt"
    juju_bootstrap "${BOOTSTRAP_PROVIDER}" "${name}" "misc" "${file}"

    END_TIME=$(date +%s)
    echo "====> Bootstrapped destination juju ($((END_TIME-START_TIME))s)"
}
