run_deploy_bundle() {
    echo

    file="${TEST_DIR}/test-bundles-deploy.txt"

    ensure "test-bundles-deploy" "${file}"

    juju deploy cs:~juju-qa/bundle/basic-0
    wait_for "ubuntu" ".applications | keys[0]"
    wait_for "ubuntu-lite" ".applications | keys[1]"

    destroy_model "test-bundles-deploy"
}

run_deploy_cmr_bundle() {
    echo

    file="${TEST_DIR}/test-cmr-bundles-deploy.txt"

    ensure "test-cmr-bundles-deploy" "${file}"

    juju deploy mysql
    wait_for "mysql" ".applications | keys[0]"

    juju offer mysql:db
    juju add-model other

    juju switch other

    bundle=./tests/suites/deploy/bundles/cmr_bundles_test_deploy.yaml
    sed "s/{{BOOTSTRAPPED_JUJU_CTRL_NAME}}/${BOOTSTRAPPED_JUJU_CTRL_NAME}/g" "${bundle}" > "${TEST_DIR}/cmr_bundles_test_deploy.yaml"
    juju deploy "${TEST_DIR}/cmr_bundles_test_deploy.yaml"

    destroy_model "test-cmr-bundles-deploy"
    destroy_model "other"
}

run_deploy_exported_bundle() {
    echo

    file="${TEST_DIR}/test-export-bundles-deploy.txt"

    ensure "test-export-bundles-deploy" "${file}"

    bundle=./tests/suites/deploy/bundles/telegraf_bundle.yaml
    juju deploy ${bundle}

    # no need to wait for the bundle to finish deploying to
    # check the export.
    juju export-bundle --filename "${TEST_DIR}/exported-bundle.yaml"
    diff ${bundle} "${TEST_DIR}/exported-bundle.yaml"

    destroy_model test-export-bundles-deploy
}

run_deploy_trusted_bundle() {
    echo

    file="${TEST_DIR}/test-trusted-bundles-deploy.txt"

    ensure "test-trusted-bundles-deploy" "${file}"

    bundle=./tests/suites/deploy/bundles/trusted_bundle.yaml
    OUT=$(juju deploy ${bundle} 2>&1 || true)
    echo "${OUT}" | check "repeat the deploy command with the --trust argument"

    juju deploy --trust ${bundle}

    wait_for "trust-checker" "$(idle_condition "trust-checker")"

    destroy_model test-trusted-bundles-deploy
}

# run_deploy_lxd_profile_bundle_openstack is to test a more
# real world scenario of a minimal openstack bundle with a
# charm using an lxd profile.
run_deploy_lxd_profile_bundle_openstack() {
    echo

    model_name="test-deploy-lxd-profile-bundle-o7k"
    file="${TEST_DIR}/${model_name}.txt"

    ensure "${model_name}" "${file}"

    bundle=cs:~juju-qa/bundle/basic-openstack-lxd-0
    juju deploy "${bundle}"

    wait_for "mysql" "$(idle_condition "mysql" 3)"
    wait_for "rabbitmq-server" "$(idle_condition "rabbitmq-server" 9)"
    wait_for "glance" "$(idle_condition "glance" 0)"
    wait_for "keystone" "$(idle_condition "keystone" 1)"
    wait_for "neutron-api" "$(idle_condition "neutron-api" 4)"
    wait_for "neutron-gateway" "$(idle_condition "neutron-gateway" 5)"
    wait_for "nova-compute" "$(idle_condition "nova-compute" 8)"
    wait_for "lxd" "$(idle_subordinate_condition "lxd" "nova-compute" 2)"
    wait_for "neutron-openvswitch" "$(idle_subordinate_condition "neutron-openvswitch" "nova-compute" 6)"
    wait_for "nova-cloud-controller" "$(idle_condition "nova-cloud-controller" 7)"

    lxd_profile_name="juju-${model_name}-neutron-openvswitch"
    machine_6="$(machine_path 6)"
    juju status --format=json | jq "${machine_6}" | check "${lxd_profile_name}"

    destroy_model "${model_name}"
}

# run_deploy_lxd_profile_bundle is to deploy multiple units of the
# same charm which has an lxdprofile in a bundle.  The scenario
# created by the bundle was found to produce failure cases during
# development of the lxd profile feature.
run_deploy_lxd_profile_bundle() {
    echo

    model_name="test-deploy-lxd-profile-bundle"
    file="${TEST_DIR}/${model_name}.txt"

    ensure "${model_name}" "${file}"

    bundle=./tests/suites/deploy/bundles/lxd-profile-bundle.yaml
    juju deploy "${bundle}"

    # 8 units of lxd-profile
    for i in 0 1 2 3 4 5 6 7
    do
        wait_for "lxd-profile" "$(idle_condition "lxd-profile" 0 "${i}")"
    done
    # 4 units of ubuntu
    for i in 0 1 2 3
    do
        wait_for "ubuntu" "$(idle_condition "ubuntu" 1 "${i}")"
    done

    lxd_profile_name="juju-${model_name}-lxd-profile"
    for i in 0 1 2 3
    do
        machine_n_lxd0="$(machine_container_path "${i}" "${i}"/lxd/0)"
        juju status --format=json | jq "${machine_n_lxd0}" | check "${lxd_profile_name}"
        machine_n_lxd1="$(machine_container_path "${i}" "${i}"/lxd/1)"
        juju status --format=json | jq "${machine_n_lxd1}" | check "${lxd_profile_name}"
    done

    destroy_model "${model_name}"
}

test_deploy_bundles() {
    if [ "$(skip 'test_deploy_bundles')" ]; then
        echo "==> TEST SKIPPED: deploy bundles"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_deploy_bundle"
        run "run_deploy_cmr_bundle"
        run "run_deploy_exported_bundle"
        run "run_deploy_trusted_bundle"
        case "${BOOTSTRAP_PROVIDER:-}" in
            "lxd")
                run "run_deploy_lxd_profile_bundle_openstack"
                run "run_deploy_lxd_profile_bundle"
                ;;
            "localhost")
                run "run_deploy_lxd_profile_bundle_openstack"
                run "run_deploy_lxd_profile_bundle"
                ;;
            *)
                echo "==> TEST SKIPPED: deploy_lxd_profile_bundle_openstack - tests for LXD only"
                echo "==> TEST SKIPPED: deploy_lxd_profile_bundle - tests for LXD only"
                ;;
        esac
    )
}
