test_local_charm() {
  if [ "$(skip 'test_local_charms')" ]; then
    echo "==> TEST SKIPPED: local charms tests"
    return
  fi

  echo "==> Checking for dependencies"
  check_dependencies juju

  file="${TEST_DIR}/test_local_charms.txt"
  bootstrap "test_local_charms" "${file}"
  test_cwd_no_git
  test_cwd_wrong_git
  destroy_controller "test_local_charms"
}
