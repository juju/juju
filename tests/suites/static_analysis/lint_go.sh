run_go() {
	VER=$(golangci-lint --version | tr -s ' ' | cut -d ' ' -f 4 | cut -d '.' -f 1,2)
	if [[ ${VER} != "1.64" ]] && [[ ${VER} != "v1.64" ]]; then
		(echo >&2 -e '\nError: golangci-lint version does not match 1.64. Please upgrade/downgrade to the right version.')
		exit 1
	fi
	OUT=$(golangci-lint run -c .github/golangci-lint.config.yaml 2>&1)
	if [[ -n ${OUT} ]]; then
		(echo >&2 "\\nError: linter has issues:\\n\\n${OUT}")
		exit 1
	fi
	OUT=$(golangci-lint run -c .github/golangci-lint.config.experimental.yaml 2>&1)
	if [[ -n ${OUT} ]]; then
		(echo >&2 "\\nError: experimental linter has issues:\\n\\n${OUT}")
		exit 1
	fi
}

run_go_tidy() {
	CUR_SHA=$(git show HEAD:go.sum | shasum -a 1 | awk '{ print $1 }')
	go mod tidy 2>&1
	NEW_SHA=$(cat go.sum | shasum -a 1 | awk '{ print $1 }')
	if [[ ${CUR_SHA} != "${NEW_SHA}" ]]; then
		git diff >&2
		(echo >&2 -e "\\nError: go mod sum is out of sync. Run 'go mod tidy' and commit source.")
		exit 1
	fi
}

join() {
	local IFS="$1"
	shift
	echo "$*"
}

run_govulncheck() {
	ignore=(
		# false positive vulnerability in github.com/canonical/lxd. This is resolved in lxd-5.21.2.
		# Anyway, it does not affect as we only use client-side lxc code, but the vulnerability is
		# server-side.
		# https://pkg.go.dev/vuln/GO-2024-3312
		# https://pkg.go.dev/vuln/GO-2024-3313
		"GO-2024-3312"
		"GO-2024-3313"
		# The vulnerability below is for a method not used since Juju 1.x.
		# https://pkg.go.dev/vuln/GO-2025-3798
		"GO-2025-3798"
	)
	ignoreMatcher=$(join "|" "${ignore[@]}")

	echo "Ignoring vulnerabilities: ${ignoreMatcher}"

	allVulns=$(govulncheck -format openvex "github.com/juju/juju/...")
	filteredVulns=$(echo ${allVulns} | jq -r '.statements[] | select(.status == "affected") | .vulnerability.name' | grep -vE "${ignoreMatcher}")

	if [[ -n ${filteredVulns} ]]; then
		(echo >&2 -e "\\nError: govulncheck has issues:\\n\\n${filteredVulns}")
		exit 1
	fi
}

test_static_analysis_go() {
	if [ "$(skip 'test_static_analysis_go')" ]; then
		echo "==> TEST SKIPPED: static go analysis"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run_linter "run_go"
		run_linter "run_go_tidy"

		# govulncheck static analysis
		if which govulncheck >/dev/null 2>&1; then
			run_linter "run_govulncheck"
		else
			echo "govulncheck not found, govulncheck static analysis disabled"
		fi
	)
}
