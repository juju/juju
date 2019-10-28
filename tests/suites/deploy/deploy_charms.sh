run_deploy_charm() {
    echo

    file="${TEST_DIR}/test-deploy.txt"

    ensure "test-deploy" "${file}"

    juju deploy cs:~jameinel/ubuntu-lite-7
    wait_for "ubuntu-lite" "$(idle_condition "ubuntu-lite")"

    destroy_model "test-deploy"
}

run_deploy_lxd_profile_charm() {
    echo

    file="${TEST_DIR}/test-deploy-lxd-profile.txt"

    ensure "test-deploy-lxd-profile" "${file}"

    juju deploy cs:~juju-qa/bionic/lxd-profile-without-devices-5
    wait_for "lxd-profile" "$(idle_condition "lxd-profile")"

    juju status --format=json | jq ".machines | .[\"0\"] | .[\"lxd-profiles\"] | keys[0]" | grep -q "juju-test-deploy-lxd-profile-lxd-profile"

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

    juju status --format=json | jq ".machines | .[\"0\"] | .[\"lxd-profiles\"] | keys" | grep -q "juju-test-deploy-local-lxd-profile-lxd-profile"

    # subordinates take longer to show, so use wait_for
    wait_for "juju-test-deploy-local-lxd-profile-lxd-profile-subordinate" ".machines | .[\"0\"] | .[\"lxd-profiles\"] | keys"

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
