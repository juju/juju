
test_deploy_os() {
    if [ "$(skip 'test_deploy_os')" ]; then
        echo "==> TEST SKIPPED: deploy to os"
        return
    fi

    (
        set_verbosity

        cd .. || exit



        case "${BOOTSTRAP_PROVIDER:-}" in
            "ec2")
                run "run_deploy_centos"
                ;;
            *)
                echo "==> TEST SKIPPED: deploy_centos - tests for LXD only"
                ;;
        esac
    )
}

run_deploy_centos() {
    echo

    name="test-deploy-centos"

    #
    # Images have been setup and and subscribed for juju-qa aws
    # in us-west-2.  Take care editing the details.
    #
    juju add-model "${name}" aws/us-west-2

    juju metadata add-image --series centos7 ami-0bc06212a56393ee1

    juju deploy ./tests/suites/deploy/charms/centos-dummy-sink --series centos7

    series=$(juju status --format=json | jq '.applications."dummy-sink".series')
    echo "$series" | check "centos7"

    wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

    destroy_model "${name}"
}
