test_cmr_bundles() {
    if [ -n "${SKIP_CMR_BUNDLES:-}" ]; then
        echo "==> SKIP: Asked to skip CMR bundle tests"
        return
    fi

    juju bootstrap lxd "cmr_bundles_test"
}
