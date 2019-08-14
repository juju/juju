run_func_vet() {
  OUT=$(grep -Rrn --include \*.go --exclude-dir=vendor "^$" -B1 . | \
      grep "func " | grep -v "}$" | \
      sed -E "s/^(.+\\.go)-([0-9]+)-(.+)$/\1:\2 \3/" \
      || true)
  if [ -n "${OUT}" ]; then
    printf "\\nERROR: Functions must not start with an empty line: \\n%s\\n" "${OUT}"
    exit 1
  fi
}

run_go_vet() {
  go vet -composites=false ./...
}

run_go_lint() {
  golint -set_exit_status ./
}

run_deadcode() {
  OUT=$(deadcode ./ 2>&1 || true)
  if [ -n "${OUT}" ]; then
  # shellcheck disable=SC2028
    echo "\\n${OUT}" >&2
    exit 1
  fi
}

run_misspell() {
  FILES=$(find ./* -name '*.go' -not -name '.#*' -not -name '*_mock.go' | grep -v vendor/ | grep -v acceptancetests/)
  OUT=$(misspell -source=go 2>/dev/null "$FILES" || true)
  if [ -n "${OUT}" ]; then
    printf "\\nFound some typos"
    echo "${OUT}"
    exit 1
  fi
}

run_ineffassign() {
  OUT=$(ineffassign ./ | grep -v "_test.go" | sed -E "s/^(.+src\\/github\\.com\\/juju\\/juju\\/)(.+)/\2/")
  if [ -n "${OUT}" ]; then
    printf "\\nFound some issues"
    echo "${OUT}"
    exit 1
  fi
}

run_go_fmt() {
  gofmt -w -s ./

  git add -u :/
  git diff --exit-code
}

test_static_analysis_go() {
  if [ -n "${SKIP_STATIC:-}" ]; then
    echo "==> SKIP: Asked to skip static analysis"
    return
  fi

  (
    set -e

    cd ../

    ## Functions starting by empty line
    run "func vet" run_func_vet

    ## go vet, if it exists
    if go help vet >/dev/null 2>&1; then
      run "go vet" run_go_vet
    fi

    ## golint
    if which golint >/dev/null 2>&1; then
      run "go lint" run_go_lint
    fi

    ## deadcode
    if which deadcode >/dev/null 2>&1; then
      run "deadcode" run_deadcode
    fi

    ## misspell
    if which misspell >/dev/null 2>&1; then
      run "misspell" run_misspell
    fi

    ## ineffassign
    if which ineffassign >/dev/null 2>&1; then
      run "ineffassign" run_ineffassign
    fi

    ## go fmt
    # run "gofmt" run_go_fmt
  )
}
