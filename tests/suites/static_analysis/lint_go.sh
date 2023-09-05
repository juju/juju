run_go() {
	VER=$(golangci-lint --version | tr -s ' ' | cut -d ' ' -f 4 | cut -d '.' -f 1,2)
	if [[ ${VER} != "1.53" ]] && [[ ${VER} != "v1.53" ]]; then
		(echo >&2 -e '\nError: golangci-lint version does not match 1.53. Please upgrade/downgrade to the right version.')
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

run_go_fanout() {
	# Ensure that the following binaries don't import each other, or are not
	# imported by any other package outside of their own package.
	for cmd in "containeragent" "jujuc" "jujud"; do
		LIST=$(find . -type f -name "*.go" | sort -u | xargs grep -EH "github\.com\/juju\/juju\/cmd\/$cmd(\/|\")" | grep -v "^./cmd/$cmd")
		if [[ -n ${LIST} ]]; then
			(echo >&2 -e "\\nError: $cmd binary is being used outside of it's package. Refactor the following list:\\n\\n${LIST}")
			exit 1
		fi
	done

	# Ensure the following packages aren't used outside of the cmd directory.
	for pkg in "modelcmd"; do
		LIST=$(find . -type f -name "*.go" | sort -u | xargs grep -EH "github\.com\/juju\/juju\/cmd\/$pkg(\/|\")" | grep -v "^./cmd")
		if [[ -n ${LIST} ]]; then
			(echo >&2 -e "\\nError: $pkg package can not be used outside of the cmd package. Refactor the following list:\\n\\n${LIST}")
			exit 1
		fi
	done
}

test_static_analysis_go() {
	if [ "$(skip 'test_static_analysis_go')" ]; then
		echo "==> TEST SKIPPED: static go analysis"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		#run_linter "run_go"
		run_linter "run_go_tidy"
		run_linter "run_go_fanout"
	)
}
