run_network_health() {
  echo

  file="${TEST_DIR}/network-health.txt"

  ensure "network-health" "${file}"

  # Deploy/relate charms and run connection tests.

  destroy_model "network-health"
}

test_network_health() {
  if [ "$(skip 'test_active_branch_output')" ]; then
    echo "==> TEST SKIPPED: test_network_health"
    return
  fi

  (
    set_verbosity

    cd .. || exit

    run "run_network_health"
  )
}
