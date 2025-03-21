run_cmr_bundles_export_overlay() {
	echo

	file="${TEST_DIR}/test-cmr-bundles-export-overlay.log"

	ensure "cmr-bundles-test-export-overlay" "${file}"

	juju add-user bar
	juju deploy ./tests/suites/deploy/bundles/bundle-with-overlays/easyrsa.yaml

# TODO(gfouillet) - recover from 3.6, delete whenever export bundle is restored or deleted
    got=$(juju export-bundle 2>&1 1>/dev/null)
    if [[ "$got" != *"not implemented"* ]]; then
        echo "ERROR: export-bundle should return 'not implemented'."
        exit 1
    fi

	juju deploy ./tests/suites/deploy/bundles/bundle-with-overlays/easyrsa-etcd.yaml --overlay overlay.yaml
# TODO(gfouillet) - recover from 3.6, delete whenever export bundle is restored or deleted
    got=$(juju export-bundle 2>&1 1>/dev/null)
    if [[ "$got" != *"not implemented"* ]]; then
        echo "ERROR: export-bundle should return 'not implemented'."
        exit 1
    fi

	destroy_model "cmr-bundles-test-export-overlay"
	destroy_model "test1"
}

test_cmr_bundles_export_overlay() {
	if [ "$(skip 'test_cmr_bundles_export_overlay')" ]; then
		echo "==> TEST SKIPPED: CMR bundle deploy tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_cmr_bundles_export_overlay"
	)
}
