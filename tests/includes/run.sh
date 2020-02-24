run() {
  CMD="${1}"

  if [ -n "${RUN_SUBTEST}" ]; then
    # shellcheck disable=SC2143
    if [ ! "$(echo "${RUN_SUBTEST}" | grep -E "^${CMD}$")" ]; then
        echo "SKIPPING: ${RUN_SUBTEST} ${CMD}"
        exit 0
    fi
  fi

  DESC=$(echo "${1}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")

  echo "===> [   ] Running: ${DESC}"

  START_TIME=$(date +%s)
  set_test_verbosity
  $CMD "$@" > "${TEST_DIR}/${TEST_CURRENT}.log" 2>&1
  case "${VERBOSE}" in
  2)
      cat "${TEST_DIR}/${TEST_CURRENT}.log"
      ;;
  3)
      cat "${TEST_DIR}/${TEST_CURRENT}.log"
      ;;
  esac
  set_verbosity
  END_TIME=$(date +%s)

  tput cuu1
  tput el1

  echo "===> [ $(green "✔") ] Success: ${DESC} ($((END_TIME-START_TIME))s)"
}

skip() {
  CMD="${1}"
  # shellcheck disable=SC2143,SC2046
  if [ $(echo "${SKIP_LIST:-}" | grep -w "${CMD}") ]; then
      echo "SKIP"
      exit 1
  fi
  return
}
