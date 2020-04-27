test_caasadmission() {
    if [ "$(skip 'test_caasadmission')" ]; then
        echo "==> TEST SKIPPED: caas admission tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju jq petname microk8s

    test_deploy_admission
}
