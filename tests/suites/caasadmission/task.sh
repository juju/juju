test_caasadmission() {
    if [ "$(skip 'test_caasadmission')" ]; then
        echo "==> TEST SKIPPED: caas admission tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju jq petname microk8s

    run_deploy_microk8s "$(petname)"

    test_controller_model_admission
    test_new_model_admission
}
