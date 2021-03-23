run_go() {
  VER=$(golangci-lint --version | tr -s ' ' | cut -d ' ' -f 4 | cut -d '.' -f 1,2)
  if [ "${VER}" != "1.36" ]; then
      (>&2 echo -e "\\nError: golangci-lint version does not match 1.36. Please upgrade/downgrade to the right version.")
      exit 1
  fi
  golangci-lint run -c .github/golangci-lint.config.yaml
  golangci-lint run -c .github/golangci-lint.config.experimental.yaml
}

run_go_tidy() {
	CUR_SHA=$(git show HEAD:go.sum | shasum -a 1 | awk '{ print $1 }')
	go mod tidy 2>&1
	NEW_SHA=$(cat go.sum | shasum -a 1 | awk '{ print $1 }')
	if [ "${CUR_SHA}" != "${NEW_SHA}" ]; then
		(echo >&2 -e "\\nError: go mod sum is out of sync. Run 'go mod tidy' and commit source.")
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
  )
}
