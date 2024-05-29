run_compileall() {
	cp -R scripts "${TEST_DIR}/"

	CURRENT_DIRECTORY=$(pwd)
	cd "${TEST_DIR}" || exit
	OUT=$(python3 -m compileall scripts -q 2>&1 || true)
	cd "${CURRENT_DIRECTORY}" || exit

	if [ -n "${OUT}" ]; then
		echo ""
		echo "$(red 'Found some issues:')"
		echo "${OUT}"
		exit 1
	fi
}

test_static_analysis_python() {
	if [ "$(skip 'test_static_analysis_python')" ]; then
		echo "==> TEST SKIPPED: static python analysis"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		# Shell static analysis
		if which python3 >/dev/null 2>&1; then
			run_linter "run_compileall"
		else
			echo "python3 not found, python static analysis disabled"
		fi
	)
}
