run_shellcheck() {
	OUT=$(shellcheck --shell=bash tests/main.sh tests/includes/*.sh tests/suites/**/*.sh 2>&1 || true)
	if [ -n "${OUT}" ]; then
		echo ""
		echo "$(red 'Found some issues:')"
		echo "${OUT}"
		exit 1
	fi
}

run_shfmt() {
	# shellcheck disable=SC2038
	OUT=$(find ./tests -type f -name "*.sh" | xargs -I% shfmt -l -s % | wc -l | grep "0" || echo "FAILED")

	if [[ ${OUT} == "FAILED" ]]; then
		echo ""
		echo "$(red 'Found some issues:')"
		# shellcheck disable=SC2038
		echo "$(find ./tests -type f -name "*.sh" | xargs -I% shfmt -l -s %)"
		exit 1
	fi
}

run_trailing_whitespace() {
	# Ensure we capture filename.sh and linenumber and nothing else.
	# filename.sh:<linenumber>:filename.sh<error>
	# shellcheck disable=SC2063
	OUT=$(grep -n -r --include "*.sh" " $" tests/ | grep -v "tmp\.*" | grep -oP "^.*:\d+" || true)
	if [ -n "${OUT}" ]; then
		echo ""
		echo "$(red 'Found some issues:')"
		echo "trailing whitespace in script"
		echo "${OUT}"
		exit 1
	fi
}

test_static_analysis_shell() {
	if [ "$(skip 'test_static_analysis_shell')" ]; then
		echo "==> TEST SKIPPED: static shell analysis"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		# Shell static analysis
		if which shellcheck >/dev/null 2>&1; then
			run_linter "run_shellcheck"
		else
			echo "shellcheck not found, shell static analysis disabled"
		fi

		# shfmt static analysis
		if which shfmt >/dev/null 2>&1; then
			run_linter "run_shfmt"
		else
			echo "shfmt not found, shell static analysis disabled"
		fi

		## Trailing whitespace in scripts
		run_linter "run_trailing_whitespace"
	)
}
