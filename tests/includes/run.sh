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

  echo -n "===> [   ] Running: ${DESC}"

  START_TIME=$(date +%s)
  set_verbosity
  $CMD "$@" | OUTPUT "${TEST_DIR}/${TEST_CURRENT}.log"
  set_verbosity
  END_TIME=$(date +%s)

  echo "\r===> [ $(green "✔") ] Success: ${DESC} ($((END_TIME-START_TIME))s)"
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
