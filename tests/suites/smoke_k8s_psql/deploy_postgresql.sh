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
    wait_for "postgresql-k8s" "$(idle_condition "postgresql-k8s")"
    wait_for "postgresql-test-app" "$(idle_condition "postgresql-test-app")"

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