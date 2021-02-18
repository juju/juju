run_local_deploy() {
    echo

    file="${2}"

    ensure "test-local-deploy" "${file}"

    juju deploy ./tests/suites/smoke/charms/ubuntu
    wait_for "ubuntu" "$(idle_condition "ubuntu")"

    juju refresh ubuntu --path=./tests/suites/smoke/charms/ubuntu

    # Wait for the refresh to happen and then wait again.
    sleep 10
    wait_for "ubuntu" "$(idle_condition "ubuntu")"

    destroy_model "test-local-deploy"
}

run_charmstore_deploy() {
    echo

    file="${2}"

    ensure "test-charmstore-deploy" "${file}"

    juju deploy cs:~jameinel/ubuntu-lite-6 ubuntu
    wait_for "ubuntu" "$(idle_condition "ubuntu")"

    juju refresh ubuntu
    wait_for "ubuntu" "$(idle_condition_for_rev "ubuntu" "9")"

    destroy_model "test-charmstore-deploy"
}


test_deploy() {
    if [ "$(skip 'test_deploy')" ]; then
        echo "==> TEST SKIPPED: smoke deploy tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        file="${1}"

        # Check that deploy runs on LXD§
        run "run_local_deploy" "${file}"
        run "run_charmstore_deploy" "${file}"
    )
}

idle_condition_for_rev() {
    local name rev app_index unit_index

    name=${1}
    rev=${2}
    app_index=${3:-0}
    unit_index=${4:-0}

    path=".[\"$name\"] | .units | .[\"$name/$unit_index\"]"

    echo ".applications | select(($path | .[\"juju-status\"] | .current == \"idle\") and ($path | .[\"workload-status\"] | .current != \"error\") and (.[\"$name\"] | .[\"charm-rev\"] == $rev)) | keys[$app_index]"
}