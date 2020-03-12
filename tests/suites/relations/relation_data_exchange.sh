run_relation_data_exchange() {
    echo

    model_name="test-relation-data-exchange"
    file="${TEST_DIR}/${model_name}.txt"

    ensure "${model_name}" "${file}"

    # Deploy 2 wordpress instances and one mysql instance
    juju deploy wordpress -n 2
    wait_for "wordpress" "$(idle_condition "wordpress" 0 0)"
    wait_for "wordpress" "$(idle_condition "wordpress" 0 1)"
    juju deploy mysql
    wait_for "mysql" "$(idle_condition "mysql")"

    # Establish relation
    juju relate wordpress mysql

    # Block until the relation is joined; otherwise, the relation-set commands
    # below will fail
    attempt=0
    while true; do
       got=$(juju run --unit 'wordpress/0' 'relation-get --app -r db:2 origin wordpress' || echo 'NOT FOUND')
       if [ "${got}" != "NOT FOUND" ]; then
         break
       fi
       attempt=$((attempt+1))
       if [ $attempt -eq 30 ]; then
         # shellcheck disable=SC2046
         echo $(red "timeout: wordpress has not yet joined db relation after 30sec")
         exit 1
       fi
       sleep 1
    done
    attempt=0
    while true; do
       got=$(juju run --unit 'mysql/0' 'relation-get --app -r db:2 origin mysql' || echo 'NOT FOUND')
       if [ "${got}" != "NOT FOUND" ]; then
         break
       fi
       attempt=$((attempt+1))
       if [ $attempt -eq 30 ]; then
         # shellcheck disable=SC2046
         echo $(red "timeout: mysql has not yet joined db relation after 30sec")
         exit 1
       fi
       sleep 1
    done

    juju run --unit 'mysql/0' 'relation-set --app -r db:2 origin=mysql'

    # As the leader units, set some *application* data for both sides of a
    # non-peer relation
    juju run --unit 'wordpress/0' 'relation-set --app -r db:2 origin=wordpress'
    juju run --unit 'mysql/0' 'relation-set --app -r db:2 origin=mysql'

    # As the leader wordpress unit, also set *application* data for a peer relation
    juju run --unit 'wordpress/0' 'relation-set --app -r loadbalancer:0 visible=to-peers'

    # Check 1: ensure that leaders can read the application databag for their
    # own application (LP1854348)
    got=$(juju run --unit 'wordpress/0' 'relation-get --app -r db:2 origin wordpress')
    if [ "${got}" != "wordpress" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected wordpress leader to read its own databag for non-peer relation")
      exit 1
    fi
    got=$(juju run --unit 'mysql/0' 'relation-get --app -r db:2 origin mysql')
    if [ "${got}" != "mysql" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected mysql leader to read its own databag for non-peer relation")
      exit 1
    fi

    # Check 2: ensure that any unit can read its own application databag for
    # *peer* relations LP1865229)
    got=$(juju run --unit 'wordpress/0' 'relation-get --app -r loadbalancer:0 visible wordpress')
    if [ "${got}" != "to-peers" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected wordpress leader to read its own databag for a peer relation")
      exit 1
    fi
    got=$(juju run --unit 'wordpress/1' 'relation-get --app -r loadbalancer:0 visible wordpress')
    if [ "${got}" != "to-peers" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected wordpress non-leader to read its own databag for a peer relation")
      exit 1
    fi

    # Check 3: ensure that non-leader units are not allowed to read their own
    # application databag for non-peer relations
    got=$(juju run --unit 'wordpress/1' 'relation-get --app -r db:2 origin wordpress' || echo 'PERMISSION DENIED')
    if [ "${got}" != "PERMISSION DENIED" ]; then
      # shellcheck disable=SC2046
      echo $(red "expected wordpress non-leader not to be allowed to read the databag for a non-peer relation")
      exit 1
    fi

    destroy_model "${model_name}"
}

test_relation_data_exchange() {
    if [ "$(skip 'test_relation_data_exchange')" ]; then
        echo "==> TEST SKIPPED: relation data exchange tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_relation_data_exchange"
    )
}
