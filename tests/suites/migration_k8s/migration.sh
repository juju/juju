#!/usr/bin/env -S bash -e

# Integration tests for k8s model migrations into a controller built from
# this branch on the same k8s cloud. The juju 3.6 scenarios bootstrap their
# source controllers with a juju 3.6 client (default /snap/bin/juju_36,
# override with JUJU_MIGRATION_36_BIN); the same-version scenario migrates
# between two controllers built from this branch.
# PostgreSQL cross-model relation migration on k8s, consuming side: the
# consumer model migrates to 4.0 while the offerer stays on 3.6. Covers
# write continuity after the move and relation-secret rotation on the
# 3.6 offerer (juju/juju#23341).
run_migration_36_cmr_secrets_consumer() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-cmr-cons36-12345.log"
	ensure "mig-target-mig-cons-12345" "${file}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-db-src-12345"
	# The consumer lives on its own 3.6 controller: juju 3.6 cannot export
	# a CAAS model that holds a same-controller SAAS ("CAAS API host ports
	# only available on the controller model"), so the offer's source
	# controller must be remote.
	bootstrap_controller_36 "mig36-cons-src-12345"

	add_model_36 "mig36-db-src-12345" "mig-db36-12345"
	${JUJU_36} switch "mig36-db-src-12345:mig-db36-12345"
	${JUJU_36} deploy postgresql-k8s --channel 14/edge/juju4 --base ubuntu@22.04 --trust --num-units 3
	${JUJU_36} deploy postgresql-test-app --channel latest/edge
	${JUJU_36} integrate postgresql-k8s postgresql-test-app:database
	${JUJU_36} offer postgresql-k8s:database db

	wait_for_36 "mig36-db-src-12345:mig-db36-12345" "postgresql-k8s" "$(active_idle_condition "postgresql-k8s")"
	wait_for_36 "mig36-db-src-12345:mig-db36-12345" "postgresql-test-app" "$(active_idle_condition "postgresql-test-app")"

	# In-model writes baseline.
	${JUJU_36} run -m "mig36-db-src-12345:mig-db36-12345" postgresql-test-app/leader start-continuous-writes \
		|| { red "start-continuous-writes failed on mig-db36-12345 (rc=$?)"; exit 1; }
	pre_local=$(mig_writes_get "${JUJU_36}" "mig36-db-src-12345:mig-db36-12345") \
		|| { red "mig_writes_get failed for mig-db36-12345 (rc=$?)"; exit 1; }
	echo "==> in-model writes baseline on mig-db36-12345: ${pre_local:-<empty>}"

	add_model_36 "mig36-cons-src-12345" "mig-cons36-12345"
	${JUJU_36} switch "mig36-cons-src-12345:mig-cons36-12345"
	${JUJU_36} deploy postgresql-test-app --channel latest/edge
	${JUJU_36} consume "mig36-db-src-12345:admin/mig-db36-12345.db"
	${JUJU_36} integrate postgresql-test-app:database db
	wait_for_36 "mig36-cons-src-12345:mig-cons36-12345" "postgresql-test-app" "$(active_idle_condition "postgresql-test-app")"
	${JUJU_36} run -m "mig36-cons-src-12345:mig-cons36-12345" postgresql-test-app/leader start-continuous-writes \
		|| { red "start-continuous-writes failed on mig-cons36-12345 (rc=$?)"; exit 1; }
	consumer_pre=$(mig_writes_get "${JUJU_36}" "mig36-cons-src-12345:mig-cons36-12345") \
		|| { red "mig_writes_get failed for mig-cons36-12345 (rc=$?)"; exit 1; }
	echo "==> consumer writes baseline on mig-cons36-12345: ${consumer_pre:-<empty>}"

	# Migrate the consuming side while the offerer stays on 3.6.
	migrate_36 "mig36-cons-src-12345" "mig-cons36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-cons-src-12345" "mig-cons36-12345"

	# Writes must resume on the migrated consumer.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-cons36-12345"
	wait_for "postgresql-test-app" "$(active_idle_condition "postgresql-test-app")"
	sleep 30
	consumer_post=$(mig_writes_get "juju" "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-cons36-12345")
	mig_assert_writes_increase "${consumer_pre}" "${consumer_post}" "${consumer_pre}" "migrated consumer ${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-cons36-12345"

	# Secret-watch continuity across the mixed-version cross-model relation
	# (juju/juju#23341): rotate the relation secret on the still-3.6
	# offerer; the migrated consumer must keep writing.
	${JUJU_36} run -m "mig36-db-src-12345:mig-db36-12345" postgresql-k8s/leader set-password
	wait_for "postgresql-test-app" "$(active_idle_condition "postgresql-test-app")"
	consumer_post2=$(mig_writes_get "juju" "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-cons36-12345")
	mig_assert_writes_increase "${consumer_post}" "${consumer_post2}" "${consumer_pre}" "migrated consumer after secret rotation"

	# The offer must be removed before model/controller destruction will
	# work. See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju remove-relation -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-cons36-12345" postgresql-test-app:database db
	juju remove-saas -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-cons36-12345" db
	${JUJU_36} remove-offer --force -y -c "mig36-db-src-12345" "admin/mig-db36-12345.db"

	# Clean up.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "mig-cons36-12345"
	destroy_controller_36 "mig36-cons-src-12345"
	destroy_controller_36 "mig36-db-src-12345"
}

# PostgreSQL cross-model relation migration on k8s, offering side: the
# offerer migrates to 4.0 with a third-party 4.0 consumer that is never
# migrated (juju/juju#23262). The in-model relation data must survive the
# import untouched (juju/juju#23337), the offer moves with its
# connections, and the PVC storage follows the model.
run_migration_36_cmr_secrets_offerer() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-cmr-offer36-12345.log"
	ensure "mig-target-mig-offer-12345" "${file}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-db-src-12345"
	bootstrap_alt_controller "mig36-hub40-12345"

	add_model_36 "mig36-db-src-12345" "mig-db36-12345"
	${JUJU_36} switch "mig36-db-src-12345:mig-db36-12345"
	${JUJU_36} deploy postgresql-k8s --channel 14/edge/juju4 --base ubuntu@22.04 --trust --num-units 3
	${JUJU_36} deploy postgresql-test-app --channel latest/edge
	${JUJU_36} integrate postgresql-k8s postgresql-test-app:database
	${JUJU_36} offer postgresql-k8s:database db

	wait_for_36 "mig36-db-src-12345:mig-db36-12345" "postgresql-k8s" "$(active_idle_condition "postgresql-k8s")"
	wait_for_36 "mig36-db-src-12345:mig-db36-12345" "postgresql-test-app" "$(active_idle_condition "postgresql-test-app")"

	# In-model writes baseline.
	${JUJU_36} run -m "mig36-db-src-12345:mig-db36-12345" postgresql-test-app/leader start-continuous-writes \
		|| { red "start-continuous-writes failed on mig-db36-12345 (rc=$?)"; exit 1; }
	pre_local=$(mig_writes_get "${JUJU_36}" "mig36-db-src-12345:mig-db36-12345") \
		|| { red "mig_writes_get failed for mig-db36-12345 (rc=$?)"; exit 1; }
	echo "==> in-model writes baseline on mig-db36-12345: ${pre_local:-<empty>}"

	# The third-party consumer on a separate 4.0 controller, never migrated.
	juju switch "mig36-hub40-12345"
	add_model "mig-hub40-12345"
	juju deploy postgresql-test-app --channel latest/edge
	juju consume "mig36-db-src-12345:admin/mig-db36-12345.db"
	juju integrate postgresql-test-app:database db
	# The third-party 4.0 consumer may fail its database-relation-changed
	# hook already under the juju 3.6 offerer (the juju/juju#23262 family):
	# tolerate both active and error here and record which happened.
	attempt=0
	until hub_status=$(${JUJU_36} status -m "mig36-hub40-12345:mig-hub40-12345" --format=json 2>/dev/null | yq -r '.applications["postgresql-test-app"].units["postgresql-test-app/0"]["workload-status"].current') && [[ ${hub_status} == "active" || ${hub_status} == "error" ]]; do
		if [[ ${attempt} -ge 60 ]]; then
			red "Failed: third-party consumer never reached active or error"
			${JUJU_36} status -m "mig36-hub40-12345:mig-hub40-12345" || true
			exit 1
		fi
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done
	hub_broken_pre="false"
	if [[ ${hub_status} == "error" ]]; then
		hub_broken_pre="true"
		echo "==> third-party consumer hook failed under the 3.6 offerer (juju/juju#23262 analog, pre-migration)"
		juju status -m "mig36-hub40-12345:mig-hub40-12345" || true
	else
		juju run -m "mig36-hub40-12345:mig-hub40-12345" postgresql-test-app/leader start-continuous-writes \
			|| { red "start-continuous-writes failed on mig-hub40-12345 (rc=$?)"; exit 1; }
		hub_pre=$(mig_writes_get "juju" "mig36-hub40-12345:mig-hub40-12345") \
			|| { red "mig_writes_get failed for mig-hub40-12345 (rc=$?)"; exit 1; }
		echo "==> third-party writes baseline on mig-hub40-12345: ${hub_pre:-<empty>}"
	fi

	# Migrate the offering side.
	migrate_36 "mig36-db-src-12345" "mig-db36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-db-src-12345" "mig-db36-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-db36-12345"
	wait_for "postgresql-k8s" "$(active_idle_condition "postgresql-k8s")"

	# The offer moved with the model and the third-party consumer is still
	# connected.
	status_out=$(juju status --format=json 2>&1 || true)
	check_contains "$(echo "${status_out}" | yq '.offers["db"]["active-connected-count"]')" "1"

	# In-model relation data must not be reset by the offering-side
	# migration (juju/juju#23337): the write counter continues, never
	# restarts.
	local_post=$(mig_writes_get "juju" "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-db36-12345")
	mig_assert_writes_ge "${pre_local}" "${local_post}" "in-model writer after offering migration"
	sleep 30
	local_post2=$(mig_writes_get "juju" "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-db36-12345")
	mig_assert_writes_increase "${pre_local}" "${local_post2}" "${pre_local}" "in-model writer ${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-db36-12345"

	# The third-party consumer on the untouched 4.0 controller must work
	# once the offerer is migrated to 4.0 (juju/juju#23262): the connection
	# is re-established against the 4.0 offerer, so a hook that failed under
	# the 3.6 offerer gets fresh data. Recover, then keep writing.
	if [[ ${hub_broken_pre} == "true" ]]; then
		attempt=0
		until hub_status2=$(juju status -m "mig36-hub40-12345:mig-hub40-12345" --format=json 2>/dev/null | yq -r '.applications["postgresql-test-app"].units["postgresql-test-app/0"]["workload-status"].current') && [[ ${hub_status2} == "active" ]]; do
			if [[ ${attempt} -ge 60 ]]; then
				red "Failed: third-party consumer did not recover after the offering migration (juju/juju#23262)"
				juju status -m "mig36-hub40-12345:mig-hub40-12345" || true
				juju exec -m "mig36-hub40-12345:mig-hub40-12345" --unit postgresql-test-app/0 -- hooks/../hooks/database-relation-changed 2>&1 | tail -5 || true
				exit 1
			fi
			sleep "${SHORT_TIMEOUT}"
			attempt=$((attempt + 1))
		done
		echo "==> third-party consumer recovered after the offering migration"
	fi
	juju run -m "mig36-hub40-12345:mig-hub40-12345" postgresql-test-app/leader start-continuous-writes \
		|| { red "start-continuous-writes failed on mig-hub40-12345 (rc=$?)"; exit 1; }
	hub_post=$(mig_writes_get "juju" "mig36-hub40-12345:mig-hub40-12345") \
		|| { red "mig_writes_get failed for mig-hub40-12345 (rc=$?)"; exit 1; }
	sleep 30
	hub_post2=$(mig_writes_get "juju" "mig36-hub40-12345:mig-hub40-12345") \
		|| { red "mig_writes_get failed for mig-hub40-12345 (rc=$?)"; exit 1; }
	mig_assert_writes_increase "${hub_post}" "${hub_post2}" "${hub_post}" "third-party consumer mig36-hub40-12345:mig-hub40-12345"
	check_not_contains "$(juju status -m "mig36-hub40-12345:mig-hub40-12345" --format=json 2>&1 || true)" '"current": "error"'

	# The postgresql PVC storage must have moved with the model.
	storage_out=$(juju storage -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-db36-12345" 2>&1 || true)
	check_contains "${storage_out}" "pgdata"
	ns=$(juju models -c "${BOOTSTRAPPED_JUJU_CTRL_NAME}" --format=json 2>/dev/null | yq -r ".models[] | select(.[\"short-name\"] == \"mig-db36-12345\") | .[\"model-uuid\"]")
	check_contains "$(kubectl -n "${ns}" get pvc 2>&1 || true)" "Bound"

	# Rotate the relation secret with everything on 4.0: the third-party
	# consumer must stay healthy (juju/juju#23341).
	juju run -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-db36-12345" postgresql-k8s/leader set-password
	wait_for "postgresql-k8s" "$(active_idle_condition "postgresql-k8s")"
	hub_post3=$(mig_writes_get "juju" "mig36-hub40-12345:mig-hub40-12345") \
		|| { red "mig_writes_get failed for mig-hub40-12345 (rc=$?)"; exit 1; }
	mig_assert_writes_increase "${hub_post2}" "${hub_post3}" "${hub_post2}" "third-party consumer after secret rotation"

	# The offer must be removed before model/controller destruction will
	# work. See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju remove-relation -m "mig36-hub40-12345:mig-hub40-12345" postgresql-test-app:database db || true
	juju remove-saas -m "mig36-hub40-12345:mig-hub40-12345" db || true
	juju remove-offer "admin/mig-db36-12345.db" -c "${BOOTSTRAPPED_JUJU_CTRL_NAME}" --force -y

	# Clean up.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "mig-db36-12345"
	destroy_controller "mig36-hub40-12345"
	destroy_controller_36 "mig36-db-src-12345"
}

# Migrating a CAAS (k8s) model from one controller to another on the same k8s
# cloud. This exercises the CAAS carve-outs in the migration path: the agent
# binary metadata lookup must not require binaries that k8s never uploads to
# the object store, and the migration minions must be the unit agents (the
# only CAAS workload agents), not a per-application operator that does not
# exist on k8s.
run_migration_caas() {
	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure we have another controller available.
	bootstrap_alt_controller "alt-model-migration-caas"
	juju switch "alt-model-migration-caas"
	add_model "model-migration-caas"

	juju deploy snappass-test --revision 8 --channel stable

	wait_for "snappass-test" "$(active_idle_condition "snappass-test")"

	# Migrate the model to the other controller.
	juju migrate "model-migration-caas" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	# Wait for the model to appear on the target, then switch to it.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_for_model "model-migration-caas"
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:model-migration-caas"

	# The application must be healthy on the target and the target must own
	# the unit agent.
	wait_for "snappass-test" "$(active_idle_condition "snappass-test")"
	juju exec --unit snappass-test/0 -- hostname | grep -c snappass-test | check 1

	# The source controller must reap the model: migration moves the model,
	# it does not copy it. The reap runs after the target has activated, so
	# poll the source until the model disappears.
	juju switch "alt-model-migration-caas:controller"
	attempt=0
	until [[ -z $(juju models --format=json | yq -r '.models | .[] | select(.["short-name"] == "model-migration-caas") | .["short-name"]') ]]; do
		if [[ ${attempt} -ge 30 ]]; then
			red 'Failed: source controller did not reap model-migration-caas'
			exit 1
		fi
		echo "[+] (attempt ${attempt}) polling for source reap of model-migration-caas"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:model-migration-caas"

	# Clean up: destroy the now-empty source controller, then the migrated
	# model on the target (the framework teardown destroys the target
	# controller itself).
	destroy_controller "alt-model-migration-caas"
	destroy_model "model-migration-caas"
}


test_migration_36_cmr_secrets_consumer() {
	if [ -n "$(skip 'test_migration_36_cmr_secrets_consumer')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration CMR secrets consumer-side migration test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration CMR secrets consumer-side migration test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"k8s")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration CMR secrets consumer-side migration test needs a k8s provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_migration_36_cmr_secrets_consumer"
	)
}


test_migration_36_cmr_secrets_offerer() {
	if [ -n "$(skip 'test_migration_36_cmr_secrets_offerer')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration CMR secrets offering-side migration test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration CMR secrets offering-side migration test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"k8s")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration CMR secrets offering-side migration test needs a k8s provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_migration_36_cmr_secrets_offerer"
	)
}

test_migration_caas() {
	if [ -n "$(skip 'test_migration_caas')" ]; then
		echo "==> SKIP: Asked to skip model migration CAAS tests"
		return
	fi

	# This test migrates a k8s model between two controllers on the same k8s
	# cloud; it only makes sense on a k8s provider.
	if [ "${BOOTSTRAP_PROVIDER}" != "k8s" ]; then
		echo "==> SKIP: model migration CAAS test needs a k8s provider"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_migration_caas"
	)
}
