run_export_overlay() {
    echo

    file="${TEST_DIR}/cmr_bundles_test_export_overlay.txt"

    bootstrap lxd "cmr_bundles_test_export_overlay" "${file}"

    juju add-user bar
    juju deploy ./testcharms/charm-repo/bundle/apache2-with-offers

    juju export-bundle

    juju add-model test1

    echo -n 'my-include' > example.txt
    cat > overlay.yaml << EOT
applications:
  wordpress:
    annotations:
      raw: include-file://example.txt
      enc: include-base64://example.txt
EOT

    juju deploy ./testcharms/charm-repo/bundle/multi-doc-overlays --overlay overlay.yaml
    juju export-bundle
}

test_export_overlay() {
    if [ -n "${SKIP_CMR_BUNDLES_EXPORT_OVERLAY:-}" ]; then
        echo "==> SKIP: Asked to skip CMR bundle deploy tests"
        return
    fi

    (
        set -e

        run "export overlay"
    )
}
