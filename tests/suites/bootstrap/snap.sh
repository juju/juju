# run_bootstrap_controller_snap_path verifies that the controller snap
# transport and installation works end-to-end for the local-build case with
# an assertion file.
#
# It downloads the `juju` snap from the snap store as a stand-in for the
# not-yet-published `juju-controller` snap.  The test verifies that:
#   1. The snap and assert files are written to /var/lib/juju/snap/.
#   2. The snap is installed on the controller machine via cloud-init.
run_bootstrap_controller_snap_path() {
  echo

  local name snap_path assert_path

  name="test-bootstrap-snap-path"

  # Download a real signed snap + assert pair from the store so that the
  # embedded cloud-init payload contains a valid assertion file.  We use
  # the `juju` snap as a stand-in because `juju-controller`
  # is not yet published in the snap store.
  echo "==> Downloading juju snap from store for transport test"
  (
    cd "${TEST_DIR}" || exit 1
    snap download juju --channel=4.0/stable --basename=juju-controller 2>&1
  )

  snap_path="${TEST_DIR}/juju-controller.snap"
  assert_path="${TEST_DIR}/juju-controller.assert"

  # Verify the download produced both required files.
  if [ ! -f "${snap_path}" ]; then
    echo "ERROR: snap file not found at ${snap_path}" >&2
    exit 1
  fi
  if [ ! -f "${assert_path}" ]; then
    echo "ERROR: assert file not found at ${assert_path}" >&2
    exit 1
  fi

  echo "Flags before test start: ${JUJU_DEV_FEATURE_FLAGS:-}"

  # Enable the controller-snap feature flag and pass the snap/assert paths
  # as additional bootstrap arguments.  BOOTSTRAP_ADDITIONAL_ARGS is
  # consumed by juju_bootstrap() inside the `bootstrap` helper.
  export JUJU_DEV_FEATURE_FLAGS="controller-snap"

  echo "==> Bootstrapping with snap transport enabled and snap/assert paths provided: ${name}"
  juju bootstrap "${BOOTSTRAP_PROVIDER:-}" "${name}" \
    --controller-snap-path="${snap_path}" \
    --controller-snap-assert-path="${assert_path}"
  echo "${name}" >>"${TEST_DIR}/jujus"

  # Switch to the controller model so we can SSH to machine 0 (the
  # bootstrap/controller machine) and verify the snap files and installation.
  juju switch "${name}:controller"

  echo "==> Verifying snap files were transported to the controller machine"

  # Assert the snap and assert files are present in the snap dir.
  snap_check=$(juju exec -m controller --unit controller/0 -- ls -h /var/lib/juju/snap)
  echo "${snap_check}"
  check_contains "${snap_check}" "juju-controller.snap"
  check_contains "${snap_check}" "juju-controller.assert"

  echo "==> Verifying snap was installed on the controller machine"

  # Assert the snap was installed (snap list includes the snap name derived
  # from the downloaded snap, which is "juju" in this stand-in test).
  snap_list=$(juju exec -m controller --unit controller/0 -- snap list)
  echo "${snap_list}"
  check_contains "${snap_list}" "juju"

  # Clean up
  destroy_controller "${name}"
  export JUJU_DEV_FEATURE_FLAGS=""
}

# run_bootstrap_controller_snap_path_without_assert verifies the dangerous
# install path: when no assertion file is provided, cloud-init installs the
# snap with --dangerous.
run_bootstrap_controller_snap_path_without_assert() {
  echo

  local name snap_path

  name="test-bootstrap-snap-no-assert"

  # Download only the snap (no assert file) to exercise the --dangerous path.
  echo "==> Downloading juju snap from store (snap only, no assert)"
  (
    cd "${TEST_DIR}" || exit 1
    snap download juju --channel=4.0/stable --basename=juju-controller-noassert 2>&1
  )

  snap_path="${TEST_DIR}/juju-controller-noassert.snap"

  if [ ! -f "${snap_path}" ]; then
    echo "ERROR: snap file not found at ${snap_path}" >&2
    exit 1
  fi

  echo "Flags before test start: ${JUJU_DEV_FEATURE_FLAGS:-}"

  export JUJU_DEV_FEATURE_FLAGS="controller-snap"

  echo "==> Bootstrapping with snap transport enabled, snap path only (no assert): ${name}"
  juju bootstrap "${BOOTSTRAP_PROVIDER:-}" "${name}" \
    --controller-snap-path="${snap_path}"
  echo "${name}" >>"${TEST_DIR}/jujus"

  juju switch "${name}:controller"

  echo "==> Verifying snap file was transported to the controller machine"

  snap_check=$(juju exec -m controller --unit controller/0 -- ls -h /var/lib/juju/snap)
  echo "${snap_check}"
  check_contains "${snap_check}" "juju-controller.snap"

  echo "==> Verifying snap was installed via --dangerous on the controller machine"

  snap_list=$(juju exec -m controller --unit controller/0 -- snap list)
  echo "${snap_list}"
  check_contains "${snap_list}" "juju"

  # Clean up
  destroy_controller "${name}"
  export JUJU_DEV_FEATURE_FLAGS=""
}

# run_feature_flag_check verifies that the controller-snap feature flag gates
# the presence of the new controller snap transport bootstrap arguments.  If
# the feature flag is not set, the new arguments should not be recognized and
# should produce an error if provided.
run_feature_flag_check() {
  echo "==> Verifying controller-snap disabled feature flag"
  echo "Flags: ${JUJU_DEV_FEATURE_FLAGS-}"

  local name output

  output=$(juju bootstrap "${BOOTSTRAP_PROVIDER:-}" ctrl-flag --controller-snap-path="/tmp/test" 2>&1 >/dev/null) || true
  check_contains "${output}" "ERROR option provided but not defined: --controller-snap-path"

  output=$(juju bootstrap "${BOOTSTRAP_PROVIDER:-}" ctrl-flag --controller-snap-assert-path="/tmp/test" 2>&1 >/dev/null) || true
  check_contains "${output}" "ERROR option provided but not defined: --controller-snap-assert-path"

  output=$(juju bootstrap "${BOOTSTRAP_PROVIDER:-}" ctrl-flag --controller-snap-channel="4.0/stable" 2>&1 >/dev/null) || true
  check_contains "${output}" "ERROR option provided but not defined: --controller-snap-channel"

  output=$(juju bootstrap "${BOOTSTRAP_PROVIDER:-}" ctrl-flag --controller-snap-revision="123" 2>&1 >/dev/null) || true
  check_contains "${output}" "ERROR option provided but not defined: --controller-snap-revision"
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

    run "run_feature_flag_check"
    run "run_bootstrap_controller_snap_path"
  )
}

test_bootstrap_controller_snap_path_without_assert() {
  if [ -n "$(skip 'test_bootstrap_controller_snap_path_without_assert')" ]; then
    echo "==> SKIP: asked to skip test_bootstrap_controller_snap_path_without_assert"
    return
  fi

  if [[ ${BOOTSTRAP_PROVIDER:-} == "k8s" || ${BOOTSTRAP_PROVIDER:-} == "microk8s" ]]; then
    echo "==> TEST SKIPPED: test_bootstrap_controller_snap_path_without_assert, not supported on k8s controller"
    return
  fi

  (
    set_verbosity

    cd .. || exit

    run "run_bootstrap_controller_snap_path_without_assert"
  )
}
