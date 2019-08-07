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
      shellcheck --shell sh test/*.sh test/includes/*.sh test/suites/*.sh
    else
      echo "shellcheck not found, shell static analysis disabled"
    fi

    ## Mixed tabs/spaces in scripts
    OUT=$(grep -Pr '\t' . | grep '\.sh:' || true)
    if [ -n "${OUT}" ]; then
      echo "ERROR: mixed tabs and spaces in script: ${OUT}"
      false
    fi

    ## Trailing whitespace in scripts
    OUT=$(grep -r " $" . | grep '\.sh:' || true)
    if [ -n "${OUT}" ]; then
      echo "ERROR: trailing whitespace in script: ${OUT}"
      false
    fi
  )
}
