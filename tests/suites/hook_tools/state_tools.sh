run_state_delete_get_set() {
    echo

    model_name="test-state-delete-get-set"
    file="${TEST_DIR}/${model_name}.txt"

    ensure "${model_name}" "${file}"

    juju deploy cs:~jameinel/ubuntu-lite-7
    wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

    juju run --unit ubuntu-lite/0 'state-get | grep -q "{}"'
    juju run --unit ubuntu-lite/0 'state-set one=two'
    juju run --unit ubuntu-lite/0 'state-get | grep -q "one: two"'
    juju run --unit ubuntu-lite/0 'state-set three=four'
    juju run --unit ubuntu-lite/0 'state-get three | grep -q "four"'
    juju run --unit ubuntu-lite/0 'state-delete one'
    juju run --unit ubuntu-lite/0 'state-get | grep -q "three: four"'
    juju run --unit ubuntu-lite/0 'state-get one --strict | grep -q "ERROR \"one\" not found" || true'
    juju run --unit ubuntu-lite/0 'state-get one'

    destroy_model "${model_name}"
}

test_state_hook_tools() {
    if [ "$(skip 'test_state_hook_tools')" ]; then
        echo "==> TEST SKIPPED: state hook tools"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_state_delete_get_set"
    )
}
