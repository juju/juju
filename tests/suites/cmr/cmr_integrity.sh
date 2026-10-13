#!/usr/bin/env bash

# CMR integrity tests.
#
# These are translated from the 3.6 CMR integrity tests introduced by
# https://github.com/juju/juju/pull/23350 and
# https://github.com/juju/juju/pull/23394. The scenarios are retained,
# but the assertions are made against the 4.0 relational schema rather
# than copied from the 3.6 Mongo document dumps:
#
#   - The remoteApplications relationcount assertions are dropped: 4.0
#     does not store a denormalised relation count. Connected counts
#     are derived and already asserted via `juju status` and the
#     offers output.
#   - The orphaned relationscopes/settings and stale unitstates
#     relation-state collection assertions become checks that the
#     relation removal cascade (domain/removal/state/model/relation.go)
#     left no rows in the relation, relation_unit_setting_archive and
#     unit_state_relation tables.
#   - The applicationOfferConnections collection assertion becomes a
#     check of the offer_connection table, alongside the
#     application_remote_consumer table that holds the offering-side
#     view of a consuming application.
#
# The 3.6 tests dumped the model database with `juju dump-db`; that
# facade method is not implemented for dqlite models, so the assertions
# here query the model databases through the db REPL
# (`juju_db_repl`, see docs/reference/juju-db-repl.md) on the
# controller machine.

# model_db_scalar executes the given SQL against the named model's
# dqlite database, using the db REPL on the controller machine, and
# prints the first scalar result. Queries passed here must select a
# single value, aliased AS n.
# On connection or execution failure the string "ERROR" is printed so
# that callers (assert_db_count, assert_offer_connections) can emit a
# diagnostic rather than a misleading count mismatch.
model_db_scalar() {
	local model_name sql out
	model_name=${1}
	sql=${2}

	# The initial controller model is always named "controller" and
	# hosts machine 0, the controller machine. stdin is piped, so juju
	# ssh allocates no pty and the REPL output is plain text. The REPL
	# prompts and its "connecting..." ticker are the only noise; the
	# result scalar is the first bare number in the output.
	# Note the REPL's .switch argument is "model-<name>", so for a
	# model actually called "model-x" the argument is
	# "model-model-x".
	if ! out=$(
		juju ssh -m controller 0 bash <<EOF 2>&1
. /etc/profile.d/juju-introspection.sh
juju_db_repl <<'SQL'
.switch model-${model_name}
${sql}
.exit
SQL
EOF
	); then
		echo "ERROR"
		return
	fi

	local scalar
	scalar=$(printf '%s\n' "${out}" | tr -d '\r' |
		awk '{ v=$0; gsub(/^[[:space:]]+|[[:space:]]+$/, "", v); if (v ~ /^[0-9]+$/) { print v; exit } }')
	if [ -z "${scalar}" ]; then
		# The REPL ran but produced no numeric output; treat as an error.
		echo "ERROR"
		return
	fi
	echo "${scalar}"
}

# assert_db_count fails unless the given table in the named model holds
# exactly the expected number of rows.
assert_db_count() {
	local model_name table expected count
	model_name=${1}
	table=${2}
	expected=${3}

	count=$(model_db_scalar "${model_name}" "SELECT COUNT(*) AS n FROM ${table};")
	if [ "${count}" = "ERROR" ]; then
		echo "    FAIL: could not query ${table} in model ${model_name} (controller unreachable or REPL error)"
		return 1
	fi
	if [ "${count}" != "${expected}" ]; then
		echo "    FAIL: expected ${expected} rows in ${table} of model ${model_name}, got ${count:-none}"
		return 1
	fi
	echo "    $(green "${table} rows in ${model_name}: ${count}")"
}

# assert_offer_connections fails unless the named offer in the named
# model holds exactly the expected number of connections.
assert_offer_connections() {
	local model_name offer_name expected count
	model_name=${1}
	offer_name=${2}
	expected=${3}

	count=$(model_db_scalar "${model_name}" "SELECT COUNT(*) AS n FROM offer_connection AS oc JOIN offer AS o ON o.uuid = oc.offer_uuid WHERE o.name = '${offer_name}';")
	if [ "${count}" = "ERROR" ]; then
		echo "    FAIL: could not query offer_connection in model ${model_name} (controller unreachable or REPL error)"
		return 1
	fi
	if [ "${count}" != "${expected}" ]; then
		echo "    FAIL: expected ${expected} connections for offer ${offer_name} in model ${model_name}, got ${count:-none}"
		return 1
	fi
	echo "    $(green "connections for offer ${offer_name} in ${model_name}: ${count}")"
}

# check_consumer_cleanup asserts that a consuming model holds no CMR
# leftovers after its SAAS application and relations were torn down.
check_consumer_cleanup() {
	local model_name
	model_name=${1}

	assert_db_count "${model_name}" application_remote_offerer 0 &&
		assert_db_count "${model_name}" relation 0 &&
		assert_db_count "${model_name}" relation_unit_setting_archive 0 &&
		assert_db_count "${model_name}" unit_state_relation 0
}

# check_offerer_cleanup asserts that an offering model holds no CMR
# leftovers after its offer connections were torn down. The offer row
# itself may remain until the offer is removed.
check_offerer_cleanup() {
	local model_name
	model_name=${1}

	assert_db_count "${model_name}" offer_connection 0 &&
		assert_db_count "${model_name}" application_remote_consumer 0 &&
		assert_db_count "${model_name}" relation 0 &&
		assert_db_count "${model_name}" relation_unit_setting_archive 0 &&
		assert_db_count "${model_name}" unit_state_relation 0
}

# wait_for_db_checks polls the given check commands (assertion functions
# with their arguments, passed as space-separated strings) until they all
# pass, or fails after the given timeout in seconds.
# Each check string is split by whitespace into a function name and its
# arguments. No argument may contain a space — model names, table names,
# and expected counts all satisfy this constraint, so eval is not needed.
wait_for_db_checks() {
	local model_name timeout
	model_name=${1}
	timeout=${2}
	shift 2

	local waited=0 attempt=0 ok out o check args
	while :; do
		ok=1
		out=""
		for check in "${@}"; do
			# Split the check string into an array; no argument contains spaces.
			read -ra args <<<"${check}"
			if o=$("${args[@]}" 2>/dev/null); then
				out+="${o}"$'\n'
			else
				out+="${o}"$'\n'
				ok=0
			fi
		done
		if [ "${ok}" -eq 1 ]; then
			printf '%s' "${out}"
			return 0
		fi
		if [ "${waited}" -ge "${timeout}" ]; then
			printf '%s' "${out}"
			echo "$(red "database integrity checks for ${model_name} failed after ${timeout}s")"
			return 1
		fi
		attempt=$((attempt + 1))
		echo "[+] (attempt ${attempt}) integrity checks not yet met for ${model_name}"
		sleep "${SHORT_TIMEOUT}"
		waited=$((waited + SHORT_TIMEOUT))
	done
}

# wait_for_cmr_established waits until the offering model sees the
# consumer's connection, then returns to the consuming model. Local
# endpoint visibility alone does not establish the remote connection;
# the consuming model's remote relation worker registers the relation
# with the offering model asynchronously. Every scenario waits for the
# connection before tearing the CMR down or inspecting the offering
# model's database, otherwise the teardown assertions pass vacuously.
wait_for_cmr_established() {
	local offer_model offer_name consume_model
	offer_model=${1}
	offer_name=${2}
	consume_model=${3}

	juju switch "${offer_model}"
	wait_for 1 ".offers.\"${offer_name}\".\"total-connected-count\""
	juju switch "${consume_model}"
}

# Clean teardown leaves no leftovers in either model.
run_cmr_clean_teardown() {
	echo
	echo "=== run_cmr_clean_teardown: normal remove-relation + remove-saas + remove-offer ==="

	file="${TEST_DIR}/test-cmr-clean-teardown.log"
	ensure "model-offer-clean" "${file}"

	echo "Deploy workload and create the offer"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink dummy-offer
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Deploy consumer and relate"
	juju add-model model-consume-clean
	juju switch model-consume-clean
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-clean.dummy-offer"
	juju relate dummy-sink dummy-offer
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for_cmr_established model-offer-clean dummy-offer model-consume-clean

	echo "Clean teardown"
	juju remove-relation dummy-sink dummy-offer
	juju remove-saas dummy-offer
	wait_for null '.applications["dummy-sink"] | .relations'
	wait_for null '."application-endpoints"'

	juju switch model-offer-clean
	wait_for null '.offers."dummy-offer"."total-connected-count"'
	juju remove-offer "admin/model-offer-clean.dummy-offer" -y
	wait_for null '.offers'

	echo "Verify database integrity after clean teardown"
	wait_for_db_checks model-consume-clean 180 \
		"check_consumer_cleanup model-consume-clean" || return 1
	wait_for_db_checks model-offer-clean 180 \
		"check_offerer_cleanup model-offer-clean" || return 1

	echo "Clean up"
	destroy_model "model-offer-clean"
	destroy_model "model-consume-clean"
}

# Force remove-relation (consuming side) doesn't leave leftovers in
# either model.
run_cmr_force_remove_relation() {
	echo
	echo "=== run_cmr_force_remove_relation: --force on consuming side ==="

	file="${TEST_DIR}/test-cmr-force-remove.log"
	ensure "model-offer-force" "${file}"

	echo "Deploy workload and create the offer"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink dummy-offer
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Deploy consumer and relate"
	juju add-model model-consume-force
	juju switch model-consume-force
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-force.dummy-offer"
	juju relate dummy-sink dummy-offer
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for_cmr_established model-offer-force dummy-offer model-consume-force

	echo "Force-remove the relation on the consuming side"
	juju remove-relation dummy-sink dummy-offer --force
	wait_for null '.applications["dummy-sink"] | .relations' 300
	juju remove-saas dummy-offer
	# Wait for the saas teardown to propagate before inspecting the
	# offering model.
	wait_for null '."application-endpoints"' 300

	echo "Wait for the offering model to drain the connection"
	juju switch model-offer-force
	# The subshell contains wait_for's exit on timeout so a slow drain
	# cannot fail the test before the database checks run.
	( wait_for null '.offers."dummy-offer"."total-connected-count"' 180 ) || true

	wait_for_db_checks model-offer-force 300 \
		"check_offerer_cleanup model-offer-force" || return 1
	wait_for_db_checks model-consume-force 300 \
		"check_consumer_cleanup model-consume-force" || return 1

	echo "Clean up"
	juju remove-offer "admin/model-offer-force.dummy-offer" --force -y || true
	destroy_model "model-offer-force"
	destroy_model "model-consume-force"
}

# Force remove-relation by id (offering side) doesn't leave dangling
# relations in either model.
run_cmr_force_remove_relation_offer_side() {
	echo
	echo "=== run_cmr_force_remove_relation_offer_side: --force by id on offering side ==="

	file="${TEST_DIR}/test-cmr-force-offer-side.log"
	ensure "model-offer-relforce" "${file}"

	echo "Deploy workload and create the offer"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink dummy-offer
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Deploy consumer and relate"
	juju add-model model-consume-relforce
	juju switch model-consume-relforce
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-relforce.dummy-offer"
	juju relate dummy-sink dummy-offer
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for_cmr_established model-offer-relforce dummy-offer model-consume-relforce

	echo "Find the relation id on the offering model"
	REL_ID=$(model_db_scalar model-offer-relforce "SELECT relation_id AS n FROM relation LIMIT 1;")
	if [ -z "${REL_ID}" ]; then
		echo "$(red "could not find relation id on offering model")"
		# Tear the CMR down before destroying the models, otherwise the
		# offering model destroy stalls on the live connection.
		juju remove-relation dummy-sink dummy-offer --force 2>/dev/null || true
		juju remove-saas dummy-offer --force 2>/dev/null || true
		juju switch model-offer-relforce
		juju remove-offer "admin/model-offer-relforce.dummy-offer" --force -y 2>/dev/null || true
		juju destroy-model --force --destroy-storage --no-prompt model-offer-relforce
		juju destroy-model --force --destroy-storage --no-prompt model-consume-relforce
		return 1
	fi
	echo "Found relation id: ${REL_ID}"

	echo "Force-remove the relation by id on the offering side"
	juju remove-relation "${REL_ID}" --force
	wait_for_db_checks model-offer-relforce 300 \
		"check_offerer_cleanup model-offer-relforce" || return 1

	echo "Tear down the consuming side"
	juju switch model-consume-relforce
	juju remove-relation dummy-sink dummy-offer --force 2>/dev/null || true
	juju remove-saas dummy-offer 2>/dev/null || true
	wait_for_db_checks model-consume-relforce 300 \
		"check_consumer_cleanup model-consume-relforce" || return 1

	echo "Clean up"
	juju switch model-offer-relforce
	juju remove-offer "admin/model-offer-relforce.dummy-offer" --force -y 2>/dev/null || true
	# Forced relation removal can interrupt a running hook. The integrity
	# checks above have passed, so do not wait for those hooks in cleanup.
	juju destroy-model --force --destroy-storage --no-prompt model-offer-relforce
	juju destroy-model --force --destroy-storage --no-prompt model-consume-relforce
}

# Re-consume doesn't leave stale connections behind.
run_cmr_consume_remove_reconsume() {
	echo
	echo "=== run_cmr_consume_remove_reconsume: re-consume the same offer ==="

	file="${TEST_DIR}/test-cmr-reconsume.log"
	ensure "model-offer-recon" "${file}"

	echo "Deploy workload and create the offer"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink dummy-offer
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "First consume cycle"
	juju add-model model-consume-recon
	juju switch model-consume-recon
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-recon.dummy-offer"
	juju relate dummy-sink dummy-offer
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for_cmr_established model-offer-recon dummy-offer model-consume-recon

	echo "Remove relation and saas (first cycle)"
	juju remove-relation dummy-sink dummy-offer
	juju remove-saas dummy-offer
	wait_for null '.applications["dummy-sink"] | .relations'

	echo "Wait for the offering model to drain the connection"
	juju switch model-offer-recon
	( wait_for null '.offers."dummy-offer"."total-connected-count"' 180 ) || true

	echo "Re-consume the same offer"
	juju switch model-consume-recon
	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-recon.dummy-offer"
	juju relate dummy-sink dummy-offer
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for_cmr_established model-offer-recon dummy-offer model-consume-recon

	echo "Verify the offering model after re-consume"
	# The first cycle must not leave stale data behind: the offer has
	# exactly one connection and one remote consumer record for the new
	# cycle.
	wait_for_db_checks model-offer-recon 300 \
		"assert_offer_connections model-offer-recon dummy-offer 1" \
		"assert_db_count model-offer-recon application_remote_consumer 1" || return 1

	echo "Verify the consuming model after re-consume"
	wait_for_db_checks model-consume-recon 300 \
		"assert_db_count model-consume-recon relation 1" || return 1

	echo "Final teardown"
	juju switch model-consume-recon
	juju remove-relation dummy-sink dummy-offer
	juju remove-saas dummy-offer
	juju switch model-offer-recon
	( wait_for null '.offers."dummy-offer"."total-connected-count"' 180 ) || true
	juju remove-offer "admin/model-offer-recon.dummy-offer" -y
	wait_for_db_checks model-offer-recon 300 \
		"check_offerer_cleanup model-offer-recon" || return 1
	wait_for_db_checks model-consume-recon 300 \
		"check_consumer_cleanup model-consume-recon" || return 1

	echo "Clean up"
	destroy_model "model-offer-recon"
	destroy_model "model-consume-recon"
}

# Destroying a consuming model doesn't leave leftovers in the offering
# model.
run_cmr_destroy_consuming_model() {
	echo
	echo "=== run_cmr_destroy_consuming_model: destroy-model with active CMR ==="

	file="${TEST_DIR}/test-cmr-destroy-consume.log"
	ensure "model-offer-destroy" "${file}"

	echo "Deploy workload and create the offer"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink dummy-offer
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Deploy consumer and relate"
	juju add-model model-consume-destroy
	juju switch model-consume-destroy
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-destroy.dummy-offer"
	juju relate dummy-sink dummy-offer
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for_cmr_established model-offer-destroy dummy-offer model-consume-destroy

	echo "Destroy the consuming model (triggers DestroyWithForce on the offering proxy)"
	juju destroy-model model-consume-destroy --destroy-storage --no-prompt

	echo "Wait for the offering model to drain"
	juju switch model-offer-destroy
	( wait_for null '.offers."dummy-offer"."total-connected-count"' 300 ) || true

	echo "Verify database integrity on the offering model after consuming model destroy"
	wait_for_db_checks model-offer-destroy 300 \
		"check_offerer_cleanup model-offer-destroy" || return 1

	echo "Clean up"
	juju remove-offer "admin/model-offer-destroy.dummy-offer" --force -y 2>/dev/null || true
	destroy_model "model-offer-destroy"
}

# Goal-state degrades gracefully when a saas app is missing.
run_cmr_goal_state_missing_saas() {
	echo
	echo "=== run_cmr_goal_state_missing_saas: goal-state with missing remote app record ==="

	file="${TEST_DIR}/test-cmr-goal-state.log"
	ensure "model-offer-gs" "${file}"

	echo "Deploy workload and create the offer"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink dummy-offer
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Deploy consumer and relate"
	juju add-model model-consume-gs
	juju switch model-consume-gs
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-gs.dummy-offer"
	juju relate dummy-sink dummy-offer
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for_cmr_established model-offer-gs dummy-offer model-consume-gs

	echo "Force-destroy the saas (leaves the relation Dying, remote app record gone)"
	juju remove-saas dummy-offer --force

	# Run goal-state on the consuming unit. The facade
	# skips the missing endpoint and returns valid goal state.
	echo "Run goal-state on the consuming unit"
	if GS_OUTPUT=$(juju exec --unit dummy-sink/0 'goal-state --format=json' 2>&1); then
		GS_RC=0
	else
		GS_RC=$?
	fi

	if [ ${GS_RC} -ne 0 ]; then
		# Check whether the failure is the expected goal-state error.
		if echo "${GS_OUTPUT}" | grep -q "saas application.*not found"; then
			echo "$(red "goal-state failed with 'saas application not found'")"
			# Clean up before returning failure.
			juju remove-relation dummy-sink dummy-offer --force 2>/dev/null || true
			juju switch model-offer-gs
			juju remove-offer "admin/model-offer-gs.dummy-offer" --force -y 2>/dev/null || true
			juju destroy-model --force --destroy-storage --no-prompt model-consume-gs
			juju destroy-model --force --destroy-storage --no-prompt model-offer-gs
			return 1
		fi

		echo "$(red "goal-state failed with unexpected error: ${GS_OUTPUT}")"
		juju remove-relation dummy-sink dummy-offer --force 2>/dev/null || true
		juju switch model-offer-gs
		juju remove-offer "admin/model-offer-gs.dummy-offer" --force -y 2>/dev/null || true
		juju destroy-model --force --destroy-storage --no-prompt model-consume-gs
		juju destroy-model --force --destroy-storage --no-prompt model-offer-gs
		return 1
	fi

	echo "$(green "goal-state succeeded despite missing saas application")"

	echo "Clean up"
	juju remove-relation dummy-sink dummy-offer --force 2>/dev/null || true
	juju switch model-offer-gs
	# The connection drain is not expected to complete here: the saas
	# was force-destroyed and the offering unit is left blocked. Poll
	# briefly for diagnostics in a subshell so a timeout cannot kill the
	# test, then force-remove everything.
	( wait_for null '.offers."dummy-offer"."total-connected-count"' 60 ) || true
	juju remove-offer "admin/model-offer-gs.dummy-offer" --force -y 2>/dev/null || true
	# Forced SAAS removal can interrupt a relation hook. The goal-state
	# assertion is complete, so force-destroy the fixtures without waiting
	# for those hooks to recover.
	juju destroy-model --force --destroy-storage --no-prompt model-consume-gs
	juju destroy-model --force --destroy-storage --no-prompt model-offer-gs
}

# Removing an offer on the offering model while it is consumed must tear
# the SAAS application down on the consuming model. The consumer's
# remote relation worker sees the offer is gone, removes the remote
# application offerer locally, including any relations, and terminates.
# This exercises the terminal handling of an already-missing offer.
run_cmr_offer_removed_remotely() {
	echo
	echo "=== run_cmr_offer_removed_remotely: remove-offer --force while consumed ==="

	file="${TEST_DIR}/test-cmr-offer-removed.log"
	ensure "model-offer-gone" "${file}"

	echo "Deploy workload and create the offer"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink dummy-offer
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Deploy consumer and relate"
	juju add-model model-consume-gone
	juju switch model-consume-gone
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-gone.dummy-offer"
	juju relate dummy-sink dummy-offer
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for_cmr_established model-offer-gone dummy-offer model-consume-gone

	echo "Force-remove the offer on the offering model"
	juju switch model-offer-gone
	juju remove-offer "admin/model-offer-gone.dummy-offer" --force -y

	echo "Verify the offering model after the offer removal"
	wait_for_db_checks model-offer-gone 300 \
		"check_offerer_cleanup model-offer-gone" || return 1

	echo "Wait for the consuming model to tear the SAAS application down"
	# The saas and its relations are removed locally once the consumer's
	# worker notices the offer is gone.
	juju switch model-consume-gone
	( wait_for null '."application-endpoints"' 300 ) || true
	wait_for_db_checks model-consume-gone 300 \
		"check_consumer_cleanup model-consume-gone" || return 1

	echo "Clean up"
	destroy_model "model-offer-gone"
	destroy_model "model-consume-gone"
}

test_cmr_integrity() {
	if [ "$(skip 'test_cmr_integrity')" ]; then
		echo "==> TEST SKIPPED: CMR integrity tests"
		return
	fi

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju go

	file="${TEST_DIR}/test-cmr-integrity.log"

	bootstrap "test-cmr-integrity" "${file}"

	(
		cd .. || exit

		run "run_cmr_clean_teardown"
		run "run_cmr_force_remove_relation"
		run "run_cmr_force_remove_relation_offer_side"
		run "run_cmr_consume_remove_reconsume"
		run "run_cmr_destroy_consuming_model"
		run "run_cmr_goal_state_missing_saas"
		run "run_cmr_offer_removed_remotely"
	)

	destroy_controller "test-cmr-integrity"
}
