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

    attempt=0
    # shellcheck disable=SC2046,SC2143
    until [ $(juju models --format=json | jq -r ".models | .[] | select(.[\"short-name\"] == \"model-migration\") | .[\"short-name\"]" | grep "model-migration") ]; do
        echo "[+] (attempt ${attempt}) polling models"
        juju models | sed 's/^/    | /g'
        sleep 5
        attempt=$((attempt+1))
    done

    juju switch "alt-model-migration:model-migration"

    wait_for "ubuntu" "$(idle_condition "ubuntu")"

    # Clean up!
    destroy_model "model-migration"
    destroy_controller "alt-model-migration"
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
