test_upgrade() {
    if [ "$(skip 'test_upgrade')" ]; then
        echo "==> TEST SKIPPED: upgrade tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju jujud juju-metadata python

    test_upgrade_simplestream
}