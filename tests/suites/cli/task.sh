test_cli() {
  if [ "$(skip 'test_cli')" ]; then
    echo "==> TEST SKIPPED: local charms tests"
    return
  fi

  echo "==> Checking for dependencies"
  check_dependencies juju

  file="${TEST_DIR}/test_local_charms.txt"
  bootstrap "test_local_charms" "${file}"

  test_local_charms

  destroy_controller "test_local_charms"
}
