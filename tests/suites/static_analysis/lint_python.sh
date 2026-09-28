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

run_unittests() {
	# CURRENT_DIR is exported by main.sh as the absolute path of tests/,
	# whatever the harness cwd is at this point.
	# The selector tests need PyYAML. Install best-effort (PEP 668 managed
	# environments need --break-system-packages); if it still cannot be
	# installed the unittest run below fails loudly with the ImportError.
	python3 -c "import yaml" 2>/dev/null || \
		python3 -m pip install --quiet pyyaml 2>/dev/null || \
		python3 -m pip install --quiet --break-system-packages pyyaml \
			2>/dev/null || true

	python3 "${CURRENT_DIR}/tools/test_select_suites.py"
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
			run_linter "run_unittests"
		else
			echo "python3 not found, python static analysis disabled"
		fi
	)
}
