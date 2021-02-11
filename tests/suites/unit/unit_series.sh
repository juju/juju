run_unit_set_series() {
    # Echo out to ensure nice output to the test suite.
    echo

    # The following ensures that a bootstrap juju exists.
    file="${TEST_DIR}/test-unit-series.log"
    ensure "unit-series" "${file}"

    juju deploy cs:ubuntu --series=bionic

    wait_for "ubuntu" "$(idle_condition "ubuntu")"

    juju set-series ubuntu focal
    juju add-unit ubuntu

    wait_for "ubuntu" "$(idle_condition "ubuntu" 0 1)"

    juju status --format=json | jq -r ".machines | .[\"1\"] | .series" | grep "focal"

    destroy_model "unit-series"
}

test_unit_series() {
    if [ -n "$(skip 'test_unit_series')" ]; then
        echo "==> SKIP: Asked to skip unit series tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_unit_set_series"
    )
}