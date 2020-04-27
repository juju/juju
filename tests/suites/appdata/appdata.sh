run_appdata_basic() {
    echo

    file="${TEST_DIR}/appdata-basic.txt"

    ensure "appdata-basic" "${file}"

    juju deploy ./charms/appdata-source
    juju deploy -n 2 ./charms/appdata-sink
    juju relate appdata-source appdata-sink

    wait_for "blocked" "$(workload_status appdata-source 0).current"

    juju config appdata-source token=test-value

    # Wait for the token to arrive on each of the sink units.
    wait_for "test-value" "$(workload_status appdata-sink 0).message"
    wait_for "test-value" "$(workload_status appdata-sink 1).message"

    # Check that the token is in /var/run/appdata-sink/token on each
    # one.
    output=$(juju ssh appdata-sink/0 cat /var/run/appdata-sink/token)
    check_contains "$output" "appdata-source/0 test-value"

    output=$(juju ssh appdata-sink/1 cat /var/run/appdata-sink/token)
    check_contains "$output" "appdata-source/0 test-value"

    juju add-unit appdata-source
    juju remove-unit appdata-source/0

    wait_for "idle" "$(agent_status appdata-source 1).current"
    juju config appdata-source token=value2

    wait_for "value2" "$(workload_status appdata-sink 0).message"
    wait_for "value2" "$(workload_status appdata-sink 1).message"

    output=$(juju ssh appdata-sink/0 cat /var/run/appdata-sink/token)
    check_contains "$output" "appdata-source/1 value2"

    output=$(juju ssh appdata-sink/1 cat /var/run/appdata-sink/token)
    check_contains "$output" "appdata-source/1 value2"

    destroy_model "appdata-basic"
}

test_appdata_int() {
  if [ "$(skip 'test_appdata_int')" ]; then
    echo "==> TEST SKIPPED: appdata int tests"
    return
  fi

  (
      set_verbosity

      cd suites/appdata || exit

      run "run_appdata_basic"
  )
}
