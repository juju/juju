#!/usr/bin/env -S bash -e

# Helpers shared by the migration suites (migration and migration_k8s)
# for migrating juju 3.6 models into a controller built from this
# branch (4.0). The source controllers are bootstrapped with a juju 3.6
# client binary (default /snap/bin/juju_36, override with
# JUJU_MIGRATION_36_BIN); the migration target is the suite's own 4.0
# controller. All clients share ~/.local/share/juju, so a 3.6 client
# and the 4.0 client see the same controllers and credentials.

JUJU_36="${JUJU_MIGRATION_36_BIN:-/snap/bin/juju_36}"

# bootstrap_controller_36 bootstraps a juju 3.6 source controller on the
# same cloud and region as the suite's 4.0 target controller. The controller
# is tracked in ${TEST_DIR}/mig36-controllers and cleaned up with the 3.6
# client (the framework's destroy_controller would fail against a 3.6
# controller). It is NOT appended to ${TEST_DIR}/jujus: the framework
# cleanup destroys controllers listed there with the 4.0 client.
bootstrap_controller_36() {
	local name cloud_region bootstrap_args attempt start_time elapsed

	name=${1}

	case "${BOOTSTRAP_PROVIDER}" in
	"lxd")
		cloud_region="${BOOTSTRAP_CLOUD:-localhost}"
		;;
	"ec2")
		cloud_region="aws/${BOOTSTRAP_REGION:-us-east-1}"
		;;
	"k8s")
		cloud_region="${BOOTSTRAP_CLOUD}"
		;;
	*)
		red "juju 3.6 migration tests are not supported on provider ${BOOTSTRAP_PROVIDER}"
		exit 1
		;;
	esac

	START_TIME=$(date +%s)
	echo "====> Bootstrapping juju 3.6 controller ${name} (${cloud_region})"
	file="${TEST_DIR}/${name}-bootstrap36.log"

	local bootstrap_args=()
	if [[ -n ${BOOTSTRAP_ARCH:-} ]]; then
		bootstrap_args+=(--bootstrap-constraints "arch=${BOOTSTRAP_ARCH}")
	fi
	# Match .github/workflows/migrate.yml: migration logging on every model
	# and no OS upgrade noise in the controller model.
	if ! ${JUJU_36} bootstrap "${cloud_region}" "${name}" \
		"${bootstrap_args[@]}" \
		--model-default logging-config="#migration=TRACE" \
		--model-default enable-os-upgrade=false \
		>"${file}" 2>&1; then
		red "Failed: bootstrapping juju 3.6 controller ${name}"
		cat "${file}"
		exit 1
	fi
	echo "${name}" >>"${TEST_DIR}/mig36-controllers"

	# Keep the migration logging explicit: --model-default only guarantees
	# defaults for hosted models.
	${JUJU_36} model-config -m "${name}:controller" "logging-config=#migration=TRACE" >/dev/null

	# Tail the controller model logs like the framework does for its own
	# controllers, so failures can be introspected from ${TEST_DIR}.
	${JUJU_36} debug-log -m "${name}:controller" --replay --tail >"${TEST_DIR}/${name}-controller-debug.log" 2>&1 &
	CMD_PID=$!
	track_daemon_pid "${CMD_PID}"

	# Wait for the controller machine agent (no machines on k8s controllers).
	if [[ ${BOOTSTRAP_PROVIDER} != "k8s" ]]; then
		attempt=0
		start_time="$(date -u +%s)"
		until [[ "$(${JUJU_36} show-machine -m "${name}:controller" 0 --format=json 2>/dev/null | yq -r '.machines["0"]["juju-status"].current')" == "started" ]]; do
			echo "[+] (attempt ${attempt}) polling for controller machine of ${name}"
			sleep "${SHORT_TIMEOUT}"

			elapsed=$(date -u +%s)-$start_time
			if [[ ${elapsed} -ge 300 ]]; then
				red "Failed: controller machine of juju 3.6 controller ${name} never started"
				${JUJU_36} status -m "${name}:controller" || true
				exit 1
			fi

			attempt=$((attempt + 1))
		done
	fi

	END_TIME=$(date +%s)
	echo "====> Bootstrapped juju 3.6 controller ${name} ($((END_TIME - START_TIME))s)"
}

# add_model_36 adds a model to a juju 3.6 controller, switches to it, keeps
# the migration logging at TRACE and applies MODEL_ARCH like the framework's
# post_add_model does.
add_model_36() {
	local controller model

	controller=${1}
	model=${2}

	${JUJU_36} add-model -c "${controller}" "${model}"
	${JUJU_36} switch "${controller}:${model}"
	${JUJU_36} model-config -m "${controller}:${model}" "logging-config=#migration=TRACE" >/dev/null

	if [[ -n ${MODEL_ARCH:-} ]]; then
		${JUJU_36} set-model-constraints -m "${controller}:${model}" "arch=${MODEL_ARCH}"
	fi

	# Tail the model logs like the framework does, so failures can be
	# introspected from ${TEST_DIR}.
	${JUJU_36} debug-log -m "${controller}:${model}" --replay --tail >"${TEST_DIR}/${controller}-${model}-debug.log" 2>&1 &
	CMD_PID=$!
	track_daemon_pid "${CMD_PID}"
}

# wait_for_36 is wait_for scoped to a juju 3.6 model: the query is run
# against ${JUJU_36} status -m <controller:model>.
wait_for_36() {
	local name model query timeout attempt start_time elapsed

	model=${1}
	name=${2}
	query=${3}
	timeout=${4:-600} # default timeout: 600s = 10m

	attempt=0
	start_time="$(date -u +%s)"
	# shellcheck disable=SC2046,SC2143
	until [[ "$(${JUJU_36} status -m "${model}" --format=json 2>/dev/null | yq "${query}" | grep "${name}")" ]]; do
		echo "[+] (attempt ${attempt}) polling status for" "${query} => ${name}"
		${JUJU_36} status -m "${model}" --relations 2>&1 | sed 's/^/    | /g'
		sleep "${SHORT_TIMEOUT}"

		elapsed=$(date -u +%s)-$start_time
		if [[ ${elapsed} -ge ${timeout} ]]; then
			echo "[-] $(red 'timed out waiting for')" "$(red "${name}")"
			echo "    (controller) juju 3.6 debug-log output"
			${JUJU_36} debug-log -m "${model%:*}:controller" --replay --no-tail 2>&1 | sed 's/^/    | /g'
			echo "    (model) juju 3.6 debug-log output"
			${JUJU_36} debug-log -m "${model}" --replay --no-tail 2>&1 | sed 's/^/    | /g'
			exit 1
		fi

		attempt=$((attempt + 1))
	done

	if [[ ${attempt} -gt 0 ]]; then
		echo "[+] $(green 'Completed polling status for')" "$(green "${name}")"
		${JUJU_36} status -m "${model}" --relations 2>&1 | sed 's/^/    | /g'
		# Although juju reports as an idle condition, some charms require a
		# breathe period to ensure things have actually settled.
		sleep "${SHORT_TIMEOUT}"
	fi
}

# juju36_exec_output is juju_exec_output scoped to the juju 3.6 client.
juju36_exec_output() {
	local yaml_output juju_rc fail_count

	{
		yaml_output=$(${JUJU_36} exec --format yaml "$@" 2>&3 3>&-) && juju_rc=0 || juju_rc=$?
	} 3>&2

	if [[ -z ${yaml_output} ]]; then
		return "${juju_rc}"
	fi

	printf '%s\n' "${yaml_output}" |
		yq -r '.[].results.stderr // "" | select(. != "")' >&2 2>/dev/null

	fail_count=$(printf '%s\n' "${yaml_output}" |
		yq '[.[] | select(.results["return-code"] != 0)] | length' 2>/dev/null) || true

	if ((${fail_count:-0} > 0)); then
		return 1
	fi

	printf '%s\n' "${yaml_output}" | yq -r '.[].results.stdout // ""'
}

# migrate_36 runs `juju migrate` with the juju 3.6 client from the source
# controller, then polls the 4.0 client until the model appears on the
# target controller. The 3.6 migrate CLI returns success before the
# transfer completes, so the poll also watches for the migration to abort
# and fails fast with the abort reason. Pass "expect-abort" as the fourth
# argument when the abort itself is the behaviour under test (see
# run_migration_36_abort): the helper then returns as soon as the
# abort is observed instead of failing, and the caller asserts the abort
# details itself.
migrate_36() {
	local src_ctrl model target_ctrl expect_abort attempt start_time elapsed

	src_ctrl=${1}
	model=${2}
	target_ctrl=${3}
	expect_abort=${4:-}

	${JUJU_36} switch "${src_ctrl}"
	if ! ${JUJU_36} migrate "${model}" "${target_ctrl}"; then
		red "Failed: juju 3.6 migrate of ${src_ctrl}:${model} to ${target_ctrl}"
		exit 1
	fi

	start_time="$(date -u +%s)"
	attempt=0
	# shellcheck disable=SC2046,SC2143
	until [[ -n $(juju models -c "${target_ctrl}" --format=json 2>/dev/null | yq -r ".models[] | .[\"short-name\"] | select(. == \"${model}\")") ]]; do
		# The 3.6 migrate CLI returns before the transfer completes; a failed
		# transfer shows up in the source model's status notes as
		# "migrating: aborted, ...".
		if ${JUJU_36} status -m "${src_ctrl}:${model}" 2>/dev/null | grep -qF "migrating: aborted"; then
			if [[ ${expect_abort} == "expect-abort" ]]; then
				echo "[+] $(green "Migration of ${src_ctrl}:${model} aborted as expected")"
				return 0
			fi
			red "Failed: migration of ${src_ctrl}:${model} aborted"
			${JUJU_36} status -m "${src_ctrl}:${model}" || true
			echo "=== last migrationmaster log lines"
			${JUJU_36} debug-log -m "${src_ctrl}:controller" --no-tail --lines 20 2>/dev/null | grep -i migrat || true
			exit 1
		fi

		echo "[+] (attempt ${attempt}) polling for ${model} on ${target_ctrl}"
		sleep "${SHORT_TIMEOUT}"

		elapsed=$(date -u +%s)-$start_time
		if [[ ${elapsed} -ge 600 ]]; then
			red "timed out waiting for ${model} to appear on ${target_ctrl}"
			${JUJU_36} status -m "${src_ctrl}:${model}" || true
			juju models -c "${target_ctrl}" || true
			exit 1
		fi

		attempt=$((attempt + 1))
	done
	echo "[+] $(green "Completed polling for ${model} on ${target_ctrl}")"
}

# wait_source_reaped_36 polls the juju 3.6 source controller until the
# migrated model has been reaped: migration moves the model, it does not
# copy it.
wait_source_reaped_36() {
	local src_ctrl model attempt

	src_ctrl=${1}
	model=${2}
	attempt=0
	# shellcheck disable=SC2046,SC2143
	until [[ -z $(${JUJU_36} models -c "${src_ctrl}" --format=json 2>/dev/null | yq -r ".models[] | .[\"short-name\"] | select(. == \"${model}\")") ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			red "Failed: source controller ${src_ctrl} did not reap ${model}"
			${JUJU_36} models -c "${src_ctrl}" || true
			exit 1
		fi
		echo "[+] (attempt ${attempt}) polling for source reap of ${model} on ${src_ctrl}"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done
	echo "[+] $(green "Source controller ${src_ctrl} reaped ${model}")"
}

# wait_target_reaped polls the 4.0 target controller until the model has
# been reaped. Aborted-migration cleanup on the target is asynchronous:
# RemoveMigratingModel only marks the model dead and the undertaker
# deletes the model database in the background, so "juju models" may
# still list the model briefly after the abort completes.
wait_target_reaped() {
	local target_ctrl model attempt

	target_ctrl=${1}
	model=${2}
	attempt=0
	# shellcheck disable=SC2046,SC2143
	until [[ -z $(juju models -c "${target_ctrl}" --format=json 2>/dev/null | yq -r ".models[] | .[\"short-name\"] | select(. == \"${model}\")") ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			red "Failed: target controller ${target_ctrl} did not reap ${model}"
			juju models -c "${target_ctrl}" || true
			exit 1
		fi
		echo "[+] (attempt ${attempt}) polling for target reap of ${model} on ${target_ctrl}"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done
	echo "[+] $(green "Target controller ${target_ctrl} reaped ${model}")"
}

# destroy_controller_36 destroys a juju 3.6 controller with the 3.6 client.
# Idempotent: a controller that is already gone is a no-op, so cleanup runs
# are safe after the test already tore everything down.
destroy_controller_36() {
	if [[ -n ${SKIP_DESTROY} ]]; then
		echo "====> Skipping destroy juju 3.6 controller"
		return
	fi

	local name output chk

	name=${1}

	# shellcheck disable=SC2086
	OUT=$(${JUJU_36} controllers --format=json 2>/dev/null | yq 'select(.controllers) | .controllers | keys | .[]' | grep "^${name}$" || true)
	# shellcheck disable=SC2181
	if [[ -z ${OUT} ]]; then
		return
	fi

	# Unfortunately having any offers on a model, leads to failure to clean
	# up a controller.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	echo "====> Removing offers from juju 3.6 controller ($(green "${name}"))"
	remove_controller_offers_36 "${name}"
	echo "====> Removed offers from juju 3.6 controller ($(green "${name}"))"

	output="${TEST_DIR}/${name}-destroy36.log"
	echo "====> Destroying juju 3.6 controller ($(green "${name}"))"
	timeout "${DESTROY_TIMEOUT}" "${JUJU_36}" destroy-controller \
		--destroy-all-models --destroy-storage --no-prompt "${name}" >"${output}" 2>&1 || true

	chk=$(cat "${output}" | grep -i "ERROR" || true)
	if [[ -n ${chk} ]]; then
		printf '\nFound some issues destroying juju 3.6 controller\n'
		cat "${output}"
	fi

	sed -i "/^${name}$/d" "${TEST_DIR}/mig36-controllers" 2>/dev/null || true
	echo "====> Destroyed juju 3.6 controller ($(green "${name}"))"
}

# remove_controller_offers_36 removes every offer hosted on the 3.6
# controller's models, mirroring the framework's remove_controller_offers
# with the 3.6 client.
remove_controller_offers_36() {
	local name models offers model offer

	name=${1}

	models=$(${JUJU_36} models -c "${name}" --format=json 2>/dev/null | yq -r ".models[] | .[\"short-name\"] | select(. != \"controller\")" || true)
	if [[ -z ${models} ]]; then
		return
	fi
	echo "${models}" | while read -r model; do
		offers=$(${JUJU_36} offers -m "${name}:${model}" --format=json 2>/dev/null | yq -r '.[] | .["offer-url"]' || true)
		echo "${offers}" | while read -r offer; do
			if [[ -n ${offer} ]]; then
				${JUJU_36} remove-offer --force -y -c "${name}" "${offer}" || true
			fi
		done
	done
}

# cleanup_mig_controllers_36 destroys every juju 3.6 controller the suite
# tracked, with the 3.6 client. Registered as a clean-up function by every
# migration scenario.
cleanup_mig_controllers_36() {
	if [[ -f "${TEST_DIR}/mig36-controllers" ]]; then
		echo "====> Cleaning up juju 3.6 controllers"
		while read -r name; do
			[[ -z ${name} ]] && continue
			destroy_controller_36 "${name}"
		done <"${TEST_DIR}/mig36-controllers"
		rm -f "${TEST_DIR}/mig36-controllers" || true
	fi
	echo "====> Completed cleaning up juju 3.6 controllers"
}

# mig_writes_get returns the continuous-writes counter of the
# postgresql-test-app leader unit in the given model. <bin> is the juju
# client that can talk to the model's controller.
mig_writes_get() {
	local bin=${1} model=${2} out

	out=$(${bin} run -m "${model}" postgresql-test-app/leader show-continuous-writes --format=yaml 2>/dev/null || true)
	echo "${out}" | yq -r '.[] | select(.results != null) | .results["writes"]' 2>/dev/null | head -n1
}

# mig_assert_writes_increase asserts the continuous-writes counter moved
# from <previous> to <current> for <label>. bash's [[ > ]] is
# lexicographic, so the comparison is arithmetic on purpose.
mig_assert_writes_increase() {
	local previous=${1} current=${2} baseline=${3} label=${4}

	if [[ ! ${current} =~ ^[0-9]+$ || ! ${baseline} =~ ^[0-9]+$ ]]; then
		red "Failed: could not parse continuous-writes counter (current=${current:-<empty>}, expected at least ${baseline:-<empty>}) for ${label}"
		exit 1
	fi
	if ((current <= baseline)); then
		red "Failed: continuous writes did not increase for ${label} (${previous} -> ${current}, expected at least ${baseline})"
		exit 1
	fi
	echo "==> continuous writes increased: ${previous} -> ${current} (${label})"
}

# mig_assert_writes_ge asserts the continuous-writes counter is at least
# <baseline> for <label>. Used where an absolute reset must be detected but
# the counter may legitimately be below an earlier reading, e.g. across the
# offering-side migration of a relation.
mig_assert_writes_ge() {
	local baseline=${1} current=${2} label=${3}

	if [[ ! ${current} =~ ^[0-9]+$ || ! ${baseline} =~ ^[0-9]+$ ]]; then
		red "Failed: could not parse continuous-writes counter (current=${current:-<empty>}, expected at least ${baseline:-<empty>}) for ${label}"
		exit 1
	fi
	if ((current < baseline)); then
		red "Failed: continuous writes regressed for ${label} (${baseline} -> ${current})"
		exit 1
	fi
	echo "==> continuous writes at or above baseline: ${baseline} <= ${current} (${label})"
}

# arm_wrench_die_in_export arms the migrationmaster "die-in-export" wrench
# on the juju 3.6 source controller used by the abort test, which makes the
# worker fail the transfer right after the model is imported into the
# target, forcing the migration to abort. The wrench directory and file
# must be owned by the root user (the user the agents run as), hence the
# sudo.
arm_wrench_die_in_export() {
	${JUJU_36} ssh -m "mig36-abort-src-12345:controller" controller/0 \
		'sudo mkdir -p /var/lib/juju/wrench && echo "die-in-export" | sudo tee /var/lib/juju/wrench/migrationmaster >/dev/null'
	${JUJU_36} ssh -m "mig36-abort-src-12345:controller" controller/0 \
		'sudo test -f /var/lib/juju/wrench/migrationmaster && sudo grep -x "die-in-export" /var/lib/juju/wrench/migrationmaster >/dev/null' || exit 1
}

cleanup_wrench_die_in_export_36() {
	${JUJU_36} ssh -m "mig36-abort-src-12345:controller" controller/0 \
		'sudo mkdir -p /var/lib/juju/wrench && : | sudo tee /var/lib/juju/wrench/migrationmaster >/dev/null' >/dev/null 2>&1 || true
}

# mig36_gate echoes a skip reason and returns non-zero when the juju 3.6
# migration tests must be skipped on this machine.
mig36_gate() {
	if ! command -v "${JUJU_36}" >/dev/null 2>&1; then
		echo "juju 3.6 binary not found (${JUJU_36}); set JUJU_MIGRATION_36_BIN"
		return 1
	fi

	local v
	v=$(${JUJU_36} version 2>/dev/null || true)
	if [[ ${v} != 3.6* ]]; then
		echo "${JUJU_36} is not a juju 3.6 client (got ${v:-unknown})"
		return 1
	fi
}
