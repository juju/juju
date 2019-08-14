run() {
  # shellcheck disable=SC2039
  echo -n "====> Running: ${1}"
  ${2}
  # shellcheck disable=SC2059,SC2028
  echo "\r====> Success: ${1}"
}

test_static_analysis() {
    test_static_analysis_go
    test_static_analysis_shell
}
