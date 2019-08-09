run_shellcheck() {
  OUT=$(shellcheck --shell sh tests/*.sh tests/includes/*.sh tests/suites/**/*.sh 2>&1 || true)
  if [ -n "${OUT}" ]; then
    printf "\\nFound some typos"
    echo "${OUT}"
    exit 1
  fi
}

run_whitespace() {
  OUT=$(grep -Pr '\t' tests/ | grep '\.sh:' || true)
  if [ -n "${OUT}" ]; then
    echo "\\nERROR: mixed tabs and spaces in script: ${OUT}"
    exit 1
  fi
}

run_trailing_whitespace() {
  OUT=$(grep -r " $" tests/ | grep '\.sh:' || true)
  if [ -n "${OUT}" ]; then
    echo "\\nERROR: trailing whitespace in script: ${OUT}"
    exit 1
  fi
}

test_static_analysis_shell() {
  (
    set -e

    cd ../

    # Shell static analysis
    if which shellcheck >/dev/null 2>&1; then
      run "shellcheck"
    else
      echo "shellcheck not found, shell static analysis disabled"
    fi

    ## Mixed tabs/spaces in scripts
    run "whitespace"

    ## Trailing whitespace in scripts
    run "trailing whitespace"
  )
}
