test_upgrade() {
    if [ "$(skip 'test_upgrade')" ]; then
        echo "==> TEST SKIPPED: upgrade tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju jujud python3
    check_juju_dependencies metadata

    test_upgrade_simplestream
}