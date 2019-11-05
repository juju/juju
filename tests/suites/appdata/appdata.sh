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

run_appdata_basic() {
    echo

    file="${TEST_DIR}/appdata-basic.txt"

    ensure "appdata-basic" "${file}"

    juju deploy ./charms/appdata-source
    juju deploy -n 2 ./charms/appdata-sink
    juju relate appdata-source appdata-sink

    wait_for "maintenance" "$(unit_status appdata-source 0).current"

    juju config appdata-source token=test-value

    # Wait for the token to arrive on each of the sink units.
    wait_for "test-value" "$(unit_status appdata-sink 0).message"
    wait_for "test-value" "$(unit_status appdata-sink 1).message"

    # Check that the token is in /var/run/appdata-sink/token on each
    # one.
    output=$(juju ssh appdata-sink/0 cat /var/run/appdata-sink/token)
    check_contains "$output" "appdata-source/0 test-value"

    output=$(juju ssh appdata-sink/1 cat /var/run/appdata-sink/token)
    check_contains "$output" "appdata-source/0 test-value"

    destroy_model "appdata-basic"
}

unit_status() {
    local app unit

    app=$1
    unit=$2

    echo ".applications[\"$app\"].units[\"$app/$unit\"][\"workload-status\"]"
    exit 0
}
