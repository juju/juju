test_charmhub() {
      if [ "$(skip 'test_charmhub')" ]; then
        echo "==> TEST SKIPPED: Charm Hub tests"
        return
    fi

    set_verbosity

    echo "==> Checking for dependencies"
    check_dependencies juju

    file="${TEST_DIR}/test-charmhub.log"

    export JUJU_DEV_FEATURE_FLAGS="charm-hub"

    bootstrap "test-charmhub" "${file}"

    test_charmhub_find
    test_charmhub_info

    destroy_controller "test-charmhub"

    unset JUJU_DEV_FEATURE_FLAGS
}