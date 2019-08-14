run_shellcheck() {
  OUT=$(shellcheck --shell sh tests/*.sh tests/includes/*.sh tests/suites/**/*.sh)
  if [ -n "${OUT}" ]; then
    printf "\\nFound some typos"
    echo "${OUT}"
    exit 1
  fi
}

test_static_analysis_shell() {
  if [ -n "${SKIP_STATIC:-}" ]; then
    echo "==> SKIP: Asked to skip static analysis"
    return
  fi

  (
    set -e

    cd ../

    # Shell static analysis
    if which shellcheck >/dev/null 2>&1; then
      run "shellcheck" run_shellcheck
    else
      echo "shellcheck not found, shell static analysis disabled"
    fi

    ## Mixed tabs/spaces in scripts
    OUT=$(grep -Pr '\t' tests/ | grep '\.sh:' || true)
    if [ -n "${OUT}" ]; then
      echo "ERROR: mixed tabs and spaces in script: ${OUT}"
      false
    fi

    ## Trailing whitespace in scripts
    OUT=$(grep -r " $" tests/ | grep '\.sh:' || true)
    if [ -n "${OUT}" ]; then
      echo "ERROR: trailing whitespace in script: ${OUT}"
      false
    fi
  )
}
