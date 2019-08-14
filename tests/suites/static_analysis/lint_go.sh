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

run_dep_check() {
  OUT=$(dep check)
  if [ -n "${OUT}" ]; then
    printf "\\nFound some issues"
    echo "\\n${OUT}" >&2
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
    printf "\\nFound some issues"
    echo "\\n${OUT}" >&2
    exit 1
  fi
}

run_misspell() {
  FILES=${2}
  OUT=$(misspell -source=go 2>/dev/null "${FILES}" || true)
  if [ -n "${OUT}" ]; then
    printf "\\nFound some issues"
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
  FILES=${2}
  OUT=$(echo "$FILES" | xargs gofmt -l -s)
  if [ -n "${OUT}" ]; then
    OUT=$(echo "${OUT}" | sed "s/^/  /")
    printf "\\nFound some issues"
    for ITEM in ${OUT}; do
      echo "gofmt -s -w ${ITEM}"
    done
    exit 1
  fi
}

test_static_analysis_go() {
  if [ -n "${SKIP_STATIC_GO:-}" ]; then
      echo "==> SKIP: Asked to skip static go analysis"
      return
  fi

  (
    set -e

    cd ../

    FILES=$(find ./* -name '*.go' -not -name '.#*' -not -name '*_mock.go' | grep -v vendor/ | grep -v acceptancetests/)

    ## Functions starting by empty line
    # turned off until we get approval of test suite
    # run "func vet"

    ## Check dependency is correct
    if which dep >/dev/null 2>&1; then
      run "dep check"
    fi

    ## go vet, if it exists
    if go help vet >/dev/null 2>&1; then
      run "go vet"
    fi

    ## golint
    if which golint >/dev/null 2>&1; then
      run "go lint"
    fi

    ## deadcode
    if which deadcode >/dev/null 2>&1; then
      run "deadcode"
    fi

    ## misspell
    if which misspell >/dev/null 2>&1; then
      run "misspell" "${FILES}"
    fi

    ## ineffassign
    # turned off until we get approval of test suite
    # if which ineffassign >/dev/null 2>&1; then
    #  run "ineffassign"
    # fi

    ## go fmt
    run "go fmt" "${FILES}"
  )
}
