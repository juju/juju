test_cmr_bundles() {
    if [ -n "${SKIP_CMR_BUNDLES:-}" ]; then
        echo "==> SKIP: Asked to skip CMR bundle tests"
        return
    fi

    test_deploy
    test_export_overlay
}
