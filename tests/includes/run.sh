run() {
  CMD="${1}"
  DESC=$(echo "${1}" | sed -E "s/^run_//g" | sed -E "s/_/ /g")

  echo -n "===> [   ] Running: ${DESC}"
  START_TIME=$(date +%s)
  $CMD "$@"
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
