run_go_vet() {
  OUT=$(go vet -composites=false ./... 2>&1 || true)
  if [ -n "${OUT}" ]; then
    echo ""
    echo "$(red 'Found some issues:')"
    echo "\\n${OUT}" >&2
    exit 1
  fi
}

test_static_analysis_go_vet() {
  if [ "$(skip 'test_static_analysis_go_vet')" ]; then
      echo "==> TEST SKIPPED: static go vet analysis"
      return
  fi

  (
    set_verbosity

    cd .. || exit

    FILES=$(find ./* -name '*.go' -not -name '.#*' -not -name '*_mock.go' | grep -v vendor/ | grep -v acceptancetests/)
    FOLDERS=$(echo "${FILES}" | sed s/^\.//g | xargs dirname | awk -F "/" '{print $2}' | uniq | sort)

    ## Functions starting by empty line
    # turned off until we get approval of test suite
    # run "func vet"

    ## go vet, if it exists
    if go help vet >/dev/null 2>&1; then
      run "run_go_vet" "${FOLDERS}"
    else
      echo "vet not found, vet static analysis disabled"
      exit 1
    fi
  )
}
