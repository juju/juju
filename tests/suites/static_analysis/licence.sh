run_licence() {
	# Before adding new licenses check with https://www.gnu.org/licenses/license-list.en.html#GPLCompatibleLicenses
	# NOTE: GPL-2.0 is not included due to the possibility it can't be relicensed under a newer version.
	go-licenses check ./... --allowed_licenses AGPL-3.0,LGPL-3.0,GPL-3.0,LGPL-2.1,Apache-2.0,0BSD,BSD-3-Clause,BSD-2-Clause,MIT,Unlicense,ISC,MPL-2.0
}

test_licence() {
	if [ "$(skip 'test_licence')" ]; then
		echo "==> TEST SKIPPED: static licence analysis"
		return
	fi
	if ! which go-licenses >/dev/null 2>&1; then
		echo "==> TEST SKIPPED: static licence analysis (go-licenses not installed)"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		# Check for licence violations.
		run_linter "run_licence"
	)
}
