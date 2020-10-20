test_bootstrap() {
    if [ "$(skip 'test_bootstrap')" ]; then
        echo "==> TEST SKIPPED: bootstrap tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju jujud go

    test_stream
}