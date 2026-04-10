# run_bootstrap_controller_snap_local verifies that the controller snap
# transport path works end-to-end for the local-build case.
#
# It downloads the `juju` snap from the snap store as a stand-in for the
# not-yet-published `juju-controller` snap.  The test is purely about
# transport: it checks that the snap and assert files are written to the
# instance's snap directory (/var/lib/juju/snap/) during bootstrap, not that
# the snap is installed or the controller starts from it.
run_bootstrap_controller_snap_path() {
  echo

  local name snap_path assert_path

  name="test-bootstrap-snap-path"

  # Download a real signed snap + assert pair from the store so that the
  # embedded cloud-init payload contains a valid assertion file.  We use
  # the `juju` snap as a stand-in because `juju-controller`
  # is not yet published in the snap store.  The content is irrelevant for
  # this transport test.
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
  # bootstrap/controller machine) and verify that the transport wrote the
  # snap files to the expected directory.
  juju switch "${name}:controller"

  echo "==> Verifying snap files were transported to the controller machine"

  # Assert the snap file is present in the snap dir.
  snap_check=$(juju exec -m controller --unit controller/0 -- ls -h /var/lib/juju/snap)
  echo "${snap_check}"
  check_contains "${snap_check}" "juju-controller.snap"
  check_contains "${snap_check}" "juju-controller.assert"

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
