test_manual() {
    if [ "$(skip 'test_manual')" ]; then
        echo "==> TEST SKIPPED: Manual tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju petname

    test_deploy_manual
}
