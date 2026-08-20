# run_bootstrap_controller_snap_path verifies that the published controller
# snap can be bootstrapped end-to-end through the snap store (channel mode).
#
# The client resolves the published `jujud` controller snap's version and
# revision from the store, then the controller machine downloads that exact
# revision itself during provisioning and cloud-init installs it from the file.
# This exercises the client-side resolution and the unified client/snap
# version-compatibility check, using --build-agent until task 4.4 pins packaged
# tools. The test verifies that:
#   1. The snap and assert files are written to /var/lib/juju/snap/.
#   2. The snap is installed and runs as `jujud`.
run_bootstrap_controller_snap_path() {
  echo

  local name

  name="test-bootstrap-snap-channel"

  echo "==> Bootstrapping controller with the published jujud snap from the store: ${name}"
  juju bootstrap "${BOOTSTRAP_PROVIDER:-}" "${name}" \
    --controller-snap-channel=latest/edge \
    --build-agent
  echo "${name}" >>"${TEST_DIR}/jujus"

  # Switch to the controller model so we can SSH to machine 0 (the
  # bootstrap/controller machine) and verify the snap files and installation.
  juju switch "${name}:controller"

  echo "==> Verifying snap files were downloaded to the controller machine"

  # The machine downloads the resolved jujud revision and its assertion to the
  # snap dir, where cloud-init installs from them.
  snap_check=$(juju exec -m controller --unit controller/0 -- ls -h /var/lib/juju/snap)
  echo "${snap_check}"
  check_contains "${snap_check}" "jujud.snap"
  check_contains "${snap_check}" "jujud.assert"

  echo "==> Verifying snap was installed on the controller machine"

  snap_list=$(juju exec -m controller --unit controller/0 -- snap list)
  echo "${snap_list}"
  check_contains "${snap_list}" "jujud"

  # Clean up
  destroy_controller "${name}"
}

test_bootstrap_controller_snap_path() {
  if [ -n "$(skip 'test_bootstrap_controller_snap_path')" ]; then
    echo "==> SKIP: asked to skip test_bootstrap_controller_snap_path"
    return
  fi

  if [[ ${BOOTSTRAP_PROVIDER:-} == "k8s" || ${BOOTSTRAP_PROVIDER:-} == "microk8s" ]]; then
    echo "==> TEST SKIPPED: test_bootstrap_controller_snap_path, not supported on k8s controller"
    return
  fi

  (
    set_verbosity

    cd .. || exit

    run "run_bootstrap_controller_snap_path"
  )
}
