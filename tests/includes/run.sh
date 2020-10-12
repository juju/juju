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

  # Prevent command from killing the script so we can capture its exit code
  # AND output. Also, make sure to grab both STDOUT and STDERR. We should be
  # using set -o pipefail here but that's unfortunately not supported by the shell.
  set_verbosity
  set +e
  cmd_output=$("${CMD}" "$@" 2>&1)
  cmd_status=$?
  echo "$cmd_output" | OUTPUT "${TEST_DIR}/${TEST_CURRENT}.log"

  set_verbosity
  END_TIME=$(date +%s)

  if [ "${cmd_status}" -eq 0 ]; then
    echo -e "\r===> [ $(green "✔") ] Success: ${DESC} ($((END_TIME-START_TIME))s)"
  else
    echo -e "\r===> [ $(red "x") ] Fail: ${DESC} ($((END_TIME-START_TIME))s)"
    exit 1
  fi
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
