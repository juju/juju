run() {
  echo -n "===> Running: ${1}"
  CMD=$(echo "${1}" | sed -E "s/ /_/g" | xargs -I % echo "run_%")
  $CMD "$@"
  echo "\r===> Success: ${1}"
}
