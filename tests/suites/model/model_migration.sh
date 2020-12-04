
# Migrating a simple one-application model from one controller to another.
run_model_migration() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists.
    file="${TEST_DIR}/test-model-migration.log"
    ensure "model-migration" "${file}"

    # Ensure we have another controller available.
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

    # Clean up.
    destroy_controller "alt-model-migration"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    destroy_model "model-migration"
}

# Migrating a model that is the offerer of a cross-model relation
# consumed by another model on the same controller.
run_model_migration_saas_common() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists.
    file="${TEST_DIR}/test-model-migration-saas-common.log"
    ensure "model-migration-saas" "${file}"

    # Ensure we have another controller available.
    bootstrap_alt_controller "alt-model-migration-saas"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju deploy ./acceptancetests/repository/charms/dummy-source
    juju offer dummy-source:sink

    wait_for "dummy-source" "$(idle_condition "dummy-source")"

    juju add-model blog
    juju switch blog
    juju deploy ./acceptancetests/repository/charms/dummy-sink

    wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

    juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
    juju relate dummy-sink dummy-source

    juju migrate "model-migration-saas" "alt-model-migration-saas"
    juju switch "alt-model-migration-saas"

    # Wait for the new model migration to appear in the alt controller.
    wait_for_model "model-migration-saas"

    # Once the model has appeared, switch to it.
    juju switch "alt-model-migration-saas:model-migration-saas"

    wait_for "dummy-source" "$(idle_condition "dummy-source")"

    # Change the dummy-source config for "token" and check that the change
    # is represented in the consuming model's dummy-sink unit.
    juju config dummy-source token=yeah-boi
    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:blog"

    wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

    # The offer must be removed before model/controller destruction will work.
    # See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
    juju switch "alt-model-migration-saas:model-migration-saas"
    juju remove-offer "admin/model-migration-saas.dummy-source" --force -y

    # Clean up.
    destroy_controller "alt-model-migration-saas"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    destroy_model "model-migration-saas"
    destroy_model "blog"
}

# Migrating a model that is the offerer of a cross-model
# relation, consumed by a model on another controller.
run_model_migration_saas_external() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists.
    file="${TEST_DIR}/test-model-migration-saas-external.log"
    ensure "model-migration-saas" "${file}"

    # Ensure we have controllers for the consuming model
    # and the migration target.
    bootstrap_alt_controller "model-migration-saas-consume"
    bootstrap_alt_controller "model-migration-saas-target"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju deploy ./acceptancetests/repository/charms/dummy-source
    juju offer dummy-source:sink

    wait_for "dummy-source" "$(idle_condition "dummy-source")"

    juju switch "model-migration-saas-consume"
    juju deploy ./acceptancetests/repository/charms/dummy-sink

    wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

    juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
    juju relate dummy-sink dummy-source

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju migrate "model-migration-saas" "model-migration-saas-target"
    juju switch "model-migration-saas-target"

    # Wait for the new model migration to appear in the target controller.
    wait_for_model "model-migration-saas"

    # Once the model has appeared, switch to it.
    juju switch "model-migration-saas"

    wait_for "dummy-source" "$(idle_condition "dummy-source")"

    # Change the dummy-source config for "token" and check that the change
    # is represented in the consuming model's dummy-sink unit.
    juju config dummy-source token=yeah-boi
    juju switch "model-migration-saas-consume"

    wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

    # The offer must be removed before model/controller destruction will work.
    # See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
    juju switch "model-migration-saas-target:model-migration-saas"
    juju remove-offer "admin/model-migration-saas.dummy-source" --force -y

    # Clean up.
    destroy_controller "model-migration-saas-consume"
    destroy_controller "model-migration-saas-target"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    destroy_model "model-migration-saas"
}

# Migrating a model that is the consumer of a cross-model
# relation, offered by a model on another controller.
run_model_migration_saas_consumer() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists.
    file="${TEST_DIR}/test-model-migration-saas-consumer.log"
    ensure "model-migration-saas" "${file}"

    # Ensure we have controllers for the consuming model
    # and the migration target.
    bootstrap_alt_controller "model-migration-saas-consume"
    bootstrap_alt_controller "model-migration-saas-target"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju deploy ./acceptancetests/repository/charms/dummy-source
    juju offer dummy-source:sink

    wait_for "dummy-source" "$(idle_condition "dummy-source")"

    juju switch "model-migration-saas-consume"
    juju add-model "model-migration-consumer"
    juju deploy ./acceptancetests/repository/charms/dummy-sink

    wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

    juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
    juju relate dummy-sink dummy-source

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju config dummy-source token=wait-for-it
    juju switch "model-migration-saas-consume"
    wait_for "wait-for-it" "$(workload_status "dummy-sink" 0).message"

    juju migrate "model-migration-consumer" "model-migration-saas-target"
    juju switch "model-migration-saas-target"

    # Wait for the new model migration to appear in the target controller.
    wait_for_model "model-migration-consumer"

    # Once the model has appeared, switch to it.
    juju switch "model-migration-consumer"

    wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

    # Change the dummy-source config for "token" and check that the change
    # is represented in the consuming model's dummy-sink unit.
    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju config dummy-source token=yeah-boi
    juju switch "model-migration-saas-target"

    wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

    # The offer must be removed before model/controller destruction will work.
    # See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju remove-offer "admin/model-migration-saas.dummy-source" --force -y

    # Clean up.
    destroy_controller "model-migration-saas-consume"
    destroy_controller "model-migration-saas-target"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    destroy_model "model-migration-saas"
}

test_model_migration() {
    if [ -n "$(skip 'test_model_migration')" ]; then
        echo "==> SKIP: Asked to skip model migration tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_model_migration"
        run "run_model_migration_saas_common"
        run "run_model_migration_saas_external"
        run "run_model_migration_saas_consumer"
    )
}

bootstrap_alt_controller() {
    local name

    name=${1}

    START_TIME=$(date +%s)
    echo "====> Bootstrapping ${name}"

    file="${TEST_DIR}/${name}.log"
    juju_bootstrap "${BOOTSTRAP_PROVIDER}" "${name}" "misc" "${file}"

    END_TIME=$(date +%s)
    echo "====> Bootstrapped ${name} ($((END_TIME-START_TIME))s)"
}
