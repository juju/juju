run_postgresql_deploy() {
    echo

    file="${2}"

    ensure "test-postgresql-deploy" "${file}"

    # Deploy the postgresql-k8s charm
    juju deploy postgresql-k8s --trust --channel 16/edge

    # Deploy the postgresql-test-app charm
    juju deploy postgresql-test-app --channel latest/edge --base ubuntu@22.04

    # Integrate the postgresql-k8s charm with the postgresql-test-app
    juju integrate postgresql-k8s postgresql-test-app:database

    # Wait for the postgresql-k8s charm to become idle
    wait_for "postgresql-k8s" "$(active_idle_condition "postgresql-k8s")"
    wait_for "postgresql-test-app" "$(active_idle_condition "postgresql-test-app")"
    wait_for "received database credentials of the first database" "$(workload_status postgresql-test-app 0).message"

    action_status=$(juju run --format json postgresql-test-app/0 start-continuous-writes \
        | jq -r '."postgresql-test-app/0".status')
    if [ "completed" != "$action_status" ]; then
        echo "ERROR: start-continous-writes action did not complete successfully"
        exit 1
    fi

    # small sleep of 3 seconds to let some writes happen to the database
    sleep 3

    action_json=$(juju run --format json postgresql-test-app/0 stop-continuous-writes \
        | jq .)
    action_status=$(echo "$action_json" | jq -r '."postgresql-test-app/0".status')
    if [ "completed" != "$action_status" ]; then
        echo "ERROR: stop-continous-writes action did not complete successfully"
        exit 1
    fi

    action_writes=$(echo "$action_json" | jq -r '."postgresql-test-app/0".results.writes')
    if [ $action_writes -le 3 ]; then
        echo "ERROR: continous write did not perform expected minimum of 3 writes"
        exit 1
    fi

    destroy_model "test-postgresql-deploy"
}

test_deploy_postgresql() {
  if [ "$(skip 'test_deploy_postgresql')" ]; then
    echo "==> TEST SKIPPED: deploy postgresql tests"
    return
  fi

  (
    set_verbosity

    cd .. || exit

    file="${1}"

    run "run_postgresql_deploy" "${file}"
  )
}
