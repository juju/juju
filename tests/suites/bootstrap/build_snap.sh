# run_build_snap_explicit verifies that --build-snap builds, uploads, and
# installs the controller snap on a LXD controller.
#
# WORKAROUND: Bootstrap is run in the background because it hangs at
# "Contacting Juju controller" — the juju-controller charm 4.1/stable
# channel does not yet publish a release for the ubuntu-core/26 base that
# the snap-built image uses.  The controller machine is provisioned and the
# snap is installed before the hang, so we poll for the controller to appear
# in "juju controllers" and then kill the background process.  Once the
# charm is published, replace the background+poll flow with a direct
# bootstrap call and verify with "juju status -m controller".
run_build_snap_explicit() {
	echo

	local name="test-build-snap-explicit"

	# Capture existing LXD containers to detect new ones after bootstrap.
	local before
	before=$(lxc list --format csv -c n 2>/dev/null | sort)

	echo "==> Bootstrapping LXD controller with --build-snap: ${name}"

	# Start bootstrap in the background.  It will hang at the
	# controller-verification step, but by then the machine is
	# provisioned and the snap is installed.
	juju bootstrap lxd "${name}" --build-snap &
	local bootstrap_pid=$!

	# Poll until the controller appears or 10 minutes elapse.
	local waited=0
	while [[ ${waited} -lt 600 ]]; do
		if juju controllers 2>/dev/null | grep -q "${name}"; then
			break
		fi
		sleep 5
		waited=$((waited + 5))
	done
	echo "${name}" >>"${TEST_DIR}/jujus"

	# Kill the background bootstrap process; it will never complete.
	kill "${bootstrap_pid}" 2>/dev/null || true
	wait "${bootstrap_pid}" 2>/dev/null || true

	echo "==> Verifying controller is registered"
	local controllers_out
	controllers_out=$(juju controllers 2>/dev/null)
	check_contains "${controllers_out}" "${name}"

	echo "==> Verifying controller snap was installed on the machine"
	local after new_container
	after=$(lxc list --format csv -c n 2>/dev/null | sort)
	new_container=$(comm -13 <(echo "${before}") <(echo "${after}") | head -1)
	if [[ -z "${new_container}" ]]; then
		echo "ERROR: could not find new LXD container for controller ${name}" >&2
		exit 1
	fi
	local snap_list
	snap_list=$(lxc exec "${new_container}" -- snap list 2>/dev/null)
	check_contains "${snap_list}" "juju-controller"

	echo "==> Cleaning up controller ${name}..."
	juju destroy-controller -y "${name}" --destroy-all-models --force --no-prompt 2>&1 || true
	lxc delete --force "${new_container}" 2>&1 || true
}

# run_build_snap_implicit verifies that bootstrap without flags
# implicitly builds, uploads, and installs the controller snap.
#
# WORKAROUND: Same background+poll approach as run_build_snap_explicit
# because of the charm channel issue.  Replace with direct bootstrap and
# "juju status -m controller" once the charm is published.
run_build_snap_implicit() {
	echo

	local name="test-build-snap-implicit"

	# Capture existing LXD containers to detect new ones after bootstrap.
	local before
	before=$(lxc list --format csv -c n 2>/dev/null | sort)

	echo "==> Bootstrapping LXD controller without flags (implicit build): ${name}"

	juju bootstrap lxd "${name}" &
	local bootstrap_pid=$!

	local waited=0
	while [[ ${waited} -lt 600 ]]; do
		if juju controllers 2>/dev/null | grep -q "${name}"; then
			break
		fi
		sleep 5
		waited=$((waited + 5))
	done
	echo "${name}" >>"${TEST_DIR}/jujus"

	kill "${bootstrap_pid}" 2>/dev/null || true
	wait "${bootstrap_pid}" 2>/dev/null || true

	echo "==> Verifying controller is registered"
	local controllers_out
	controllers_out=$(juju controllers 2>/dev/null)
	check_contains "${controllers_out}" "${name}"

	echo "==> Verifying controller snap was installed on the machine"
	local after new_container
	after=$(lxc list --format csv -c n 2>/dev/null | sort)
	new_container=$(comm -13 <(echo "${before}") <(echo "${after}") | head -1)
	if [[ -z "${new_container}" ]]; then
		echo "ERROR: could not find new LXD container for controller ${name}" >&2
		exit 1
	fi
	local snap_list
	snap_list=$(lxc exec "${new_container}" -- snap list 2>/dev/null)
	check_contains "${snap_list}" "jujud"

	echo "==> Cleaning up controller ${name}..."
	juju unregister --no-prompt "${name}" 2>&1 || true
	lxc delete --force "${new_container}" 2>&1 || true
}

# check_snapcraft verifies that snapcraft is available on $PATH.
# If not, the test exits 0 from the subshell so that the suite skips
# gracefully rather than failing.
check_snapcraft() {
	if ! command -v snapcraft &>/dev/null; then
		echo "SKIP: snapcraft not installed (required for build-snap)"
		exit 0
	fi
}

# test_bootstrap_build_snap is the top-level test entry point for the
# --build-snap integration smoke test.  It validates both explicit
# --build-snap and implicit (no flags) bootstrap flows end-to-end:
# building the snap from local source, uploading it to the LXD instance,
# and verifying installation.
#
# The test skips on k8s providers (build-snap is IAAS-only) and when
# snapcraft is not available.
test_bootstrap_build_snap() {
	if [ -n "$(skip 'test_bootstrap_build_snap')" ]; then
		echo "==> SKIP: asked to skip test_bootstrap_build_snap"
		return
	fi

	if [[ ${BOOTSTRAP_PROVIDER:-} == "k8s" || ${BOOTSTRAP_PROVIDER:-} == "microk8s" ]]; then
		echo "==> TEST SKIPPED: test_bootstrap_build_snap, not supported on k8s controller"
		return
	fi

	check_snapcraft

	(
		set_verbosity

		cd .. || exit

		run "run_build_snap_explicit"
		run "run_build_snap_implicit"
	)
}
