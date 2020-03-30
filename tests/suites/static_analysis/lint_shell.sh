run_shellcheck() {
  OUT=$(shellcheck --shell sh tests/main.sh tests/includes/*.sh tests/suites/**/*.sh 2>&1 || true)
  if [ -n "${OUT}" ]; then
    echo ""
    echo "$(red 'Found some issues:')"
    echo "${OUT}"
    exit 1
  fi
}

run_whitespace() {
  # Ensure we capture filename.sh and linenumber and nothing else.
  # filename.sh:<linenumber>:filename.sh<error>
  # shellcheck disable=SC2063
  OUT=$(grep -n -r --include "*.sh" "$(printf '\t')" tests/ | grep -oP "^.*:\d+" || true)
  if [ -n "${OUT}" ]; then
    echo ""
    echo "$(red 'Found some issues:')"
    echo "mixed tabs and spaces in script"
    echo "${OUT}"
    exit 1
  fi
}

run_trailing_whitespace() {
  # Ensure we capture filename.sh and linenumber and nothing else.
  # filename.sh:<linenumber>:filename.sh<error>
  # shellcheck disable=SC2063
  OUT=$(grep -n -r --include "*.sh" " $" tests/ | grep -oP "^.*:\d+" || true)
  if [ -n "${OUT}" ]; then
    echo ""
    echo "$(red 'Found some issues:')"
    echo "trailing whitespace in script"
    echo "${OUT}"
    exit 1
  fi
}

run_test_setup() {
  # shellcheck disable=SC2038
  OUT=$(find tests/suites -iname '*.sh' | xargs grep -rEoh '^run_\w+\s?[^\(]' | sort)
  echo "${OUT}" | while read -r subtest; do
    S=$(grep -owr "${subtest}" tests/suites)
    COUNT=$(echo "${S}" | wc -l)
    TEST_FILE=$(echo "${S}" | cut -f1 -d":")
    if ${COUNT} % 2 == 0 ; then
      echo ""
      echo "$(red 'Found some issues:')"
      echo "Expected subtest (${subtest}) to be in the same file as test (${TEST_FILE})."
      exit 1
    fi
    H=$(echo "${S}" | head -n 1)
    T=$(echo "${S}" | tail -n 1)
    if [ "${H}" != "${T}" ]; then
      echo ""
      echo "$(red 'Found some issues:')"
      echo "Expected subtest (${subtest}) to be in the same file as test (${TEST_FILE})."
      exit 1
    fi
  done
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
      run "run_shellcheck"
    else
      echo "shellcheck not found, shell static analysis disabled"
    fi

    ## Mixed tabs/spaces in scripts
    run "run_whitespace"

    ## Trailing whitespace in scripts
    run "run_trailing_whitespace"

    ## Tests are wired up
    run "run_test_setup"
  )
}
