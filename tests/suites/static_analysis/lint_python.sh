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
	# environments need --break-system-packages). If it is still not
	# importable, skip with a loud, distinct message: a missing runner
	# package is an infra gap to fix on the runner image (or PyPI access
	# to restore), not a PR defect — it must not turn the required
	# Static Analysis check red on every PR with a bare ImportError.
	python3 -c "import yaml" 2>/dev/null || \
		python3 -m pip install --quiet pyyaml 2>/dev/null || \
		python3 -m pip install --quiet --break-system-packages pyyaml \
			2>/dev/null || true

	if ! python3 -c "import yaml" >/dev/null 2>&1; then
		echo "SKIP: selector unit tests — PyYAML is not importable and"
		echo "SKIP: could not be installed on this runner. Fix the runner"
		echo "SKIP: image (python3-yaml) or PyPI access; not a PR defect."
		return 0
	fi

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
