run_deploy_charm() {
    echo

    file="${TEST_DIR}/test-deploy-charm.txt"

    ensure "test-deploy-charm" "${file}"

    juju deploy cs:~jameinel/ubuntu-lite-7
    wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

    destroy_model "test-deploy-charm"
}

run_deploy_lxd_profile_charm() {
    echo

    file="${TEST_DIR}/test-deploy-lxd-profile.txt"

    ensure "test-deploy-lxd-profile" "${file}"

    juju deploy cs:~juju-qa/bionic/lxd-profile-without-devices-5
    wait_for "lxd-profile" "$(idle_condition "lxd-profile")"

    juju status --format=json | jq ".machines | .[\"0\"] | .[\"lxd-profiles\"] | keys[0]" | check "juju-test-deploy-lxd-profile-lxd-profile"

    destroy_model "test-deploy-lxd-profile"
}

run_deploy_local_lxd_profile_charm() {
    echo

    file="${TEST_DIR}/test-deploy-local-lxd-profile.txt"

    ensure "test-deploy-local-lxd-profile" "${file}"

    juju deploy ./tests/suites/deploy/charms/lxd-profile
    juju deploy ./tests/suites/deploy/charms/lxd-profile-subordinate
    juju add-relation lxd-profile-subordinate lxd-profile

    wait_for "lxd-profile" "$(idle_condition "lxd-profile")"
    wait_for "lxd-profile-subordinate" ".applications | keys[1]"

    lxd_profile_name="juju-test-deploy-local-lxd-profile-lxd-profile"
    lxd_profile_sub_name="juju-test-deploy-local-lxd-profile-lxd-profile-subordinate"

    # subordinates take longer to show, so use wait_for
    machine_0="$(machine_path 0)"
    wait_for "${lxd_profile_sub_name}" "${machine_0}"

    juju status --format=json | jq "${machine_0}" | check "${lxd_profile_name}"
    juju status --format=json | jq "${machine_0}" | check "${lxd_profile_sub_name}"

    juju add-unit "lxd-profile"

    machine_1="$(machine_path 1)"
    wait_for "${lxd_profile_sub_name}" "${machine_1}"

    juju status --format=json | jq "${machine_1}" | check "${lxd_profile_name}"
    juju status --format=json | jq "${machine_1}" | check "${lxd_profile_sub_name}"

    juju add-unit "lxd-profile" --to lxd

    machine_2="$(machine_container_path 2 2/lxd/0)"
    wait_for "${lxd_profile_sub_name}" "${machine_2}"

    juju status --format=json | jq "${machine_2}" | check "${lxd_profile_name}"
    juju status --format=json | jq "${machine_2}" | check "${lxd_profile_sub_name}"

    destroy_model "test-deploy-local-lxd-profile"
}

test_deploy_charms() {
    if [ "$(skip 'test_deploy_charms')" ]; then
        echo "==> TEST SKIPPED: deploy charms"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_deploy_charm"
        run "run_deploy_lxd_profile_charm"
        run "run_deploy_local_lxd_profile_charm"
    )
}

machine_path() {
    local machine

    machine=${1}

    echo ".machines | .[\"${machine}\"] | .[\"lxd-profiles\"] | keys"
}

machine_container_path() {
    local machine container

    machine=${1}
    container=${2}

    echo ".machines | .[\"${machine}\"] | .containers | .[\"${container}\"] | .[\"lxd-profiles\"] | keys"
}