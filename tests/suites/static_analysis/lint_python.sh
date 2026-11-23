run_compileall() {
	rsync -r --exclude-from=acceptancetests/.gitignore acceptancetests "${TEST_DIR}/"
	cp -R scripts "${TEST_DIR}/"

	CURRENT_DIRECTORY=$(pwd)
	cd "${TEST_DIR}" || exit
	OUT=$(python3 -m compileall acceptancetests scripts -q 2>&1 || true)
	cd "${CURRENT_DIRECTORY}" || exit

	if [ -n "${OUT}" ]; then
		echo ""
		echo "$(red 'Found some issues:')"
		echo "${OUT}"
		exit 1
	fi
}

run_script_unittests() {
	# Run from a copy so __pycache__ is never written into the source tree.
	# build_metadata.py runs git status --porcelain to compute git_tree_state,
	# so stray bytecode under scripts/ would mark local snap builds as dirty.
	if [ ! -d "${TEST_DIR}/scripts/snap" ]; then
		cp -R scripts "${TEST_DIR}/"
	fi

	CURRENT_DIRECTORY=$(pwd)
	cd "${TEST_DIR}" || exit
	OUT=$(python3 -m unittest discover -s scripts/snap -p 'test_*.py' 2>&1)
	OUT_CODE=$?
	cd "${CURRENT_DIRECTORY}" || exit

	if [ ${OUT_CODE} -ne 0 ]; then
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
			run_linter "run_script_unittests"
		else
			echo "python3 not found, python static analysis disabled"
		fi
	)
}
