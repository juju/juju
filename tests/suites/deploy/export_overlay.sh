test_cmr_bundles_export_overlay() {
	if true; then
		echo "==> TEST SKIPPED: CMR bundle deploy tests, as 'export-bundle' command is not supported in the current Juju Major version."
		return
	fi

	(
		set_verbosity

		cd .. || exit
	)
}
