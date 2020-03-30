run_relation_departing_unit() {
    echo

    model_name="test-relation-departing-unit"
    file="${TEST_DIR}/${model_name}.txt"

    ensure "${model_name}" "${file}"

    # Deploy 2 departer instances
    juju deploy ./tests/suites/relations/charms/departer -n 2
    wait_for "departer" "$(idle_condition "departer" 0 0)"
    wait_for "departer" "$(idle_condition "departer" 0 1)"

    # Remove departer/1
    juju remove-unit departer/1
    sleep 5

    got=$(juju debug-log --replay --include unit-departer-0 | grep 'Remote unit departer/1 is departing the relation' || true)
    if [ -z "${got}" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected departer/0 to be notified that departer/1 went away")
      exit 1
    fi
    got=$(juju debug-log --replay --include unit-departer-1 | grep 'Local unit is departing the relation' || true)
    if [ -z "${got}" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected departer/1 to be notified that it is going away")
      exit 1
    fi

    destroy_model "${model_name}"
}

test_relation_departing_unit() {
    if [ "$(skip 'test_relation_departing_unit')" ]; then
        echo "==> TEST SKIPPED: relation departing unit tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_relation_departing_unit"
    )
}
