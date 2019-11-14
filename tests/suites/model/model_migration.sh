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

run_model_migration_saas_block() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists
    file="${TEST_DIR}/test-model-migration-saas.txt"
    ensure "model-migration-saas" "${file}"

    # Ensure we have another controller available
    bootstrap_alt_controller "alt-model-migration-saas"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju deploy mysql

    wait_for "mysql" "$(idle_condition "mysql")"

    juju offer mysql:db
    juju add-model blog

    juju switch blog

    generate_saas_bundle "${BOOTSTRAPPED_JUJU_CTRL_NAME}" "model-migration-saas"

    juju deploy "${TEST_DIR}/saas_wordpress.yaml"

    wait_for "wordpress" "$(idle_condition "wordpress")"

    juju migrate "model-migration-saas" "alt-model-migration-saas"
    juju switch "alt-model-migration-saas"

    # Wait for the new model migration to appear in the alt controller.
    wait_for_model "model-migration-saas"

    # Once the model has appeared, switch to it.
    juju switch "alt-model-migration-saas:model-migration-saas"

    wait_for "mysql" "$(idle_condition "mysql")"

    # Clean up!
    destroy_controller "alt-model-migration-saas"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    destroy_model "model-migration-saas"
    destroy_model "blog"
}

run_model_migration_saas_consumer() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists
    file="${TEST_DIR}/test-model-migration-consume.txt"
    ensure "model-migration-consume" "${file}"

    # Ensure we have another controller available
    bootstrap_alt_controller "alt-model-migration-consume"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    juju deploy mysql

    wait_for "mysql" "$(idle_condition "mysql")"

    juju offer mysql:db
    juju add-model blog

    juju switch blog

    generate_saas_bundle "${BOOTSTRAPPED_JUJU_CTRL_NAME}" "model-migration-consume"

    juju deploy "${TEST_DIR}/saas_wordpress.yaml"

    wait_for "wordpress" "$(idle_condition "wordpress")"

    juju migrate "blog" "alt-model-migration-consume"
    juju switch "alt-model-migration-consume"

    # Wait for the new model migration to appear in the alt controller.
    wait_for_model "blog"

    # Once the model has appeared, switch to it.
    juju switch "alt-model-migration-consume:blog"

    wait_for "wordpress" "$(idle_condition "wordpress")"

    juju expose wordpress

    # Clean up!
    destroy_controller "alt-model-migration-consume"

    juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
    destroy_model "model-migration-consume"
    destroy_model "blog"
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
        run "run_model_migration_saas_block"
        run "run_model_migration_saas_consumer"
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

generate_saas_bundle() {
    local ctrl_name model_name

    ctrl_name=${1}
    model_name=${2}

    bundle=./tests/suites/model/bundles/saas_wordpress.yaml
    cp "${bundle}" "${TEST_DIR}/saas_wordpress.yaml"

    sed -i "s/{{BOOTSTRAPPED_JUJU_CTRL_NAME}}/${ctrl_name}/g" "${TEST_DIR}/saas_wordpress.yaml"
    sed -i "s/{{JUJU_MODEL_NAME}}/${model_name}/g" "${TEST_DIR}/saas_wordpress.yaml"
}