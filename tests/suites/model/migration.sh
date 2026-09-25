#!/usr/bin/env -S bash -e

# Integration tests for migrating juju 3.6 models into a controller built
# from this branch (4.0). The source controllers are bootstrapped with a
# juju 3.6 client binary (default /snap/bin/juju_36, override with
# JUJU_MIGRATION_36_BIN); the migration target is the suite's own 4.0
# controller.
#
# The scenarios pin down the cross-model relation, secrets, spaces and
# --via behaviours reported in juju/juju#23262 and juju/juju#23337 to
# #23344. Assertions are strict: they stay red until each fix lands.

# Resolve the juju 3.6 binary used to bootstrap and drive source
# controllers. All clients share ~/.local/share/juju, so a 3.6 client and
# the 4.0 client see the same controllers and credentials.
JUJU_36="${JUJU_MIGRATION_36_BIN:-/snap/bin/juju_36}"

# Migrating a 3.6 IAAS model carrying secrets, storage, an active relation
# and logs into the 4.0 controller, then asserting the model is fully
# functional on the target.
run_model_migration_36() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-core36-12345.log"
	ensure "mig-target-core-12345" "${file}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-core-src-12345"
	add_model_36 "mig36-core-src-12345" "mig-core36-12345"

	${JUJU_36} switch "mig36-core-src-12345:mig-core36-12345"
	${JUJU_36} deploy ubuntu-lite ubuntu --base ubuntu@22.04

	# Pack the local filesystem storage charm. The packed legacy charm only
	# supports base ubuntu@24.04 under charmcraft 4, so it gets its own
	# machine rather than sharing the jammy machine 0.
	charm=$(pack_charm ./testcharms/charms/dummy-storage-fs)
	${JUJU_36} deploy "$charm" --base ubuntu@24.04 --storage data=rootfs,1G

	${JUJU_36} deploy juju-qa-dummy-source --base ubuntu@22.04
	${JUJU_36} deploy juju-qa-dummy-sink --base ubuntu@22.04
	${JUJU_36} integrate dummy-sink dummy-source

	wait_for_36 "mig36-core-src-12345:mig-core36-12345" "ubuntu" "$(idle_condition "ubuntu")"
	wait_for_36 "mig36-core-src-12345:mig-core36-12345" "dummy-source" "$(idle_condition "dummy-source")"
	wait_for_36 "mig36-core-src-12345:mig-core36-12345" "dummy-sink" "$(idle_condition "dummy-sink")"
	wait_for_36 "mig36-core-src-12345:mig-core36-12345" "dummy-storage-fs" "$(idle_condition "dummy-storage-fs")"

	# Wait for the storage attachment to come up.
	attempt=0
	until [[ -n $(${JUJU_36} storage -m "mig36-core-src-12345:mig-core36-12345" 2>/dev/null | grep "data/0" || true) ]]; do
		if [[ ${attempt} -ge 60 ]]; then
			red "Failed: storage data/0 was never attached on the source model"
			${JUJU_36} storage -m "mig36-core-src-12345:mig-core36-12345" || true
			exit 1
		fi
		echo "[+] (attempt ${attempt}) polling for storage data/0"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	# create user secrets.
	user_secret_uri=$(${JUJU_36} add-secret mysecret owned-by="model" --info "this is a user secret")
	user_secret_short_uri=${user_secret_uri##*:}
	check_contains "$(${JUJU_36} show-secret mysecret --revisions | yq ".${user_secret_short_uri}.description")" 'this is a user secret'
	${JUJU_36} grant-secret mysecret "ubuntu"
	check_contains "$(juju36_exec_output --unit "ubuntu/0" -- secret-get "$user_secret_short_uri")" "owned-by: model"

	# create charm-owned secrets.
	unit_owned_secret_uri=$(juju36_exec_output --unit ubuntu/0 -- secret-add --owner unit owned-by=ubuntu/0)
	unit_owned_secret_short_uri=${unit_owned_secret_uri##*:}
	check_contains "$(juju36_exec_output --unit "ubuntu/0" -- secret-get "$unit_owned_secret_short_uri")" "owned-by: ubuntu/0"
	app_owned_secret_uri=$(juju36_exec_output --unit ubuntu/0 -- secret-add owned-by=ubuntu)
	app_owned_secret_short_uri=${app_owned_secret_uri##*:}
	check_contains "$(juju36_exec_output --unit "ubuntu/0" -- secret-get "$app_owned_secret_short_uri")" "owned-by: ubuntu"

	# Capture logs to ensure they are migrated.
	old_logs="$(${JUJU_36} debug-log -m "mig36-core-src-12345:mig-core36-12345" --no-tail -l DEBUG)"

	migrate_36 "mig36-core-src-12345" "mig-core36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-core-src-12345" "mig-core36-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-core36-12345"

	wait_for "ubuntu" "$(idle_condition "ubuntu")"
	wait_for "dummy-source" "$(idle_condition "dummy-source")"
	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"
	wait_for "dummy-storage-fs" "$(idle_condition "dummy-storage-fs")"

	# Check that the secrets are still present and accessible.
	check_contains "$(juju show-secret mysecret --revisions | yq ".${user_secret_short_uri}.description")" 'this is a user secret'
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get "$user_secret_short_uri")" "owned-by: model"
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get "$unit_owned_secret_short_uri")" "owned-by: ubuntu/0"
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get "$app_owned_secret_short_uri")" "owned-by: ubuntu"

	# check we can still create new secrets.
	user_secret_uri1=$(juju add-secret mysecret1 owned-by="model-as-well" --info "this is another user secret")
	user_secret_short_uri1=${user_secret_uri1##*:}
	check_contains "$(juju show-secret mysecret1 --revisions | yq ".${user_secret_short_uri1}.description")" 'this is another user secret'
	unit_owned_secret_uri1=$(juju_exec_output --unit ubuntu/0 -- secret-add --owner unit owned-by=ubuntu/0)
	unit_owned_secret_short_uri1=${unit_owned_secret_uri1##*:}
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get "$unit_owned_secret_short_uri1")" "owned-by: ubuntu/0"

	# Storage must still be attached to the migrated unit.
	storage_out=$(juju storage -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-core36-12345" 2>&1 || true)
	check_contains "${storage_out}" "data/0"
	check_contains "${storage_out}" "attached"

	# Change the dummy-source config for "token" and check that the change is
	# represented in the consuming model's dummy-sink unit.
	juju config dummy-source token=yeah-boi
	wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	# Add a unit to ubuntu to ensure the model is functional.
	juju add-unit ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu" 1)"

	# Assert old logs have been transferred over.
	new_logs="$(juju debug-log --no-tail --replay -l DEBUG)"
	if [[ ${new_logs} != *"${old_logs}"* ]]; then
		echo "$(red 'logs failed to migrate')"
		echo "=== old log lines: $(echo "${old_logs}" | wc -l); new log lines: $(echo "${new_logs}" | wc -l)"
		echo "=== first old line: $(echo "${old_logs}" | head -n1)"
		first_ts=$(echo "${old_logs}" | head -n1 | sed -E 's/^[^:]+: ([0-9]{2}:[0-9]{2}:[0-9]{2}) .*/\1/')
		echo "=== new log lines carrying timestamp ${first_ts}: $(echo "${new_logs}" | grep -cF "${first_ts}" || true)"
		echo "${new_logs}" | grep -F "${first_ts}" | head -n3 || true
		exit 1
	fi

	# Clean up.
	destroy_controller_36 "mig36-core-src-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "mig-core36-12345"
}

# The offering side of a cross-model relation migrates into the 4.0
# controller while the consumer (a third party on a separate controller)
# stays put. Covers the offer ACL with an unknown external user
# (juju/juju#23342) and third-party consumer continuity (juju/juju#23262).
run_model_migration_36_cmr_offering() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-cmr-off36-12345.log"
	ensure "mig-target-cmr-off-12345" "${file}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-off-src-12345"
	bootstrap_alt_controller "mig36-peer40-12345"

	add_model_36 "mig36-off-src-12345" "mig-offer36-12345"
	${JUJU_36} switch "mig36-off-src-12345:mig-offer36-12345"
	${JUJU_36} deploy juju-qa-dummy-source --base ubuntu@22.04
	wait_for_36 "mig36-off-src-12345:mig-offer36-12345" "dummy-source" "$(idle_condition "dummy-source")"
	${JUJU_36} offer dummy-source:sink db-src
	# The dummy-source charm blocks until the token is set, which stalls the
	# cross-model relation join on the consumer side; set it before the
	# consumer integrates.
	${JUJU_36} config dummy-source token=wait-for-it

	# Grant consume access to an unknown external user. The ACL entry must
	# survive the import on the 4.0 side (juju/juju#23342): before the fix,
	# unknown users abort the import entirely.
	${JUJU_36} grant bob@external consume "mig36-off-src-12345:admin/mig-offer36-12345.db-src"

	juju switch "mig36-peer40-12345"
	add_model "mig-peer40-cons-12345"
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "mig36-off-src-12345:admin/mig-offer36-12345.db-src"
	juju integrate dummy-sink db-src
	# wait for relation joined before migrate.
	# work around for fixing:
	# ERROR source prechecks failed: unit dummy-source/0 hasn't joined
	# relation "dummy-source:sink remote-<uuid>:source" yet
	wait_for "db-src" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for "wait-for-it" "$(workload_status "dummy-sink" 0).message"
	sleep 30

	wait_for_36 "mig36-off-src-12345:mig-offer36-12345" "1" '.offers["db-src"]["active-connected-count"]'

	migrate_36 "mig36-off-src-12345" "mig-offer36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-off-src-12345" "mig-offer36-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-offer36-12345"
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	# The offer must have moved with the model, and its CMR connection must
	# have survived the offering-side move.
	status_out=$(juju status --format=json 2>&1 || true)
	check_contains "$(echo "${status_out}" | yq '.offers["db-src"]["active-connected-count"]')" "1"

	# The ACL with the unknown external user must survive (juju/juju#23342).
	# The yaml output carries the full users map; the tabular output does not.
	acl_out=$(juju show-offer "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/mig-offer36-12345.db-src" --format=yaml 2>&1 || true)
	check_contains "${acl_out}" "bob@external"
	check_contains "${acl_out}" "consume"

	# The third-party consumer, never migrated, must keep flowing
	# (juju/juju#23262 analog on IAAS).
	juju config dummy-source token=yeah-boi
	juju switch "mig36-peer40-12345:mig-peer40-cons-12345"

	wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	# The offer must be removed before model/controller destruction will
	# work. See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju remove-offer "admin/mig-offer36-12345.db-src" -c "${BOOTSTRAPPED_JUJU_CTRL_NAME}" --force -y

	# Clean up.
	destroy_controller "mig36-peer40-12345"
	destroy_controller_36 "mig36-off-src-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "mig-offer36-12345"
}

# The consuming side migrates from a third-party juju 3.6 controller.
# Covers a relation through a consume alias (juju/juju#23339), a second
# consumed offer of the same offerer (juju/juju#23338: pre-migration a 3.6
# consumer model cannot hold a second CMR connection - the uniter and
# remote-relations workers die with "permission denied"/"connection is shut
# down", so the second offer is integrated only after the migration) and a
# consumed offer with no relations at all (juju/juju#23340).
run_model_migration_36_cmr_consuming() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-cmr-cons36-12345.log"
	ensure "mig-target-cmr-cons-12345" "${file}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-off-src-12345"
	bootstrap_controller_36 "mig36-peer36-12345"

	add_model_36 "mig36-off-src-12345" "mig-offer36-12345"
	${JUJU_36} switch "mig36-off-src-12345:mig-offer36-12345"
	${JUJU_36} deploy juju-qa-dummy-source --base ubuntu@22.04
	${JUJU_36} deploy juju-qa-dummy-source ds2 --base ubuntu@22.04
	wait_for_36 "mig36-off-src-12345:mig-offer36-12345" "dummy-source" "$(idle_condition "dummy-source")"
	wait_for_36 "mig36-off-src-12345:mig-offer36-12345" "ds2" "$(idle_condition "ds2")"
	${JUJU_36} offer dummy-source:sink db-src
	${JUJU_36} offer ds2:sink db-src2

	add_model_36 "mig36-peer36-12345" "mig-cons36-12345"
	${JUJU_36} switch "mig36-peer36-12345:mig-cons36-12345"
	${JUJU_36} deploy juju-qa-dummy-sink --base ubuntu@22.04

	wait_for_36 "mig36-peer36-12345:mig-cons36-12345" "dummy-sink" "$(idle_condition "dummy-sink")"

	# The dummy-source charm blocks until the token is set, which stalls the
	# cross-model relation join; set the tokens before integrating.
	${JUJU_36} config -m "mig36-off-src-12345:mig-offer36-12345" dummy-source token=pre-mig
	${JUJU_36} config -m "mig36-off-src-12345:mig-offer36-12345" ds2 token=pre-mig
	# Consume the offer under the "secondary" alias and the second offer of
	# the same offerer under "other". Only ONE relation may be integrated on
	# the juju 3.6 consumer (a second CMR connection to the same offerer
	# kills the consumer's workers, juju/juju#23338), so "other" stays
	# consume-only until after the migration.
	${JUJU_36} consume "mig36-off-src-12345:admin/mig-offer36-12345.db-src" secondary
	${JUJU_36} consume "mig36-off-src-12345:admin/mig-offer36-12345.db-src2" other
	${JUJU_36} integrate dummy-sink secondary

	wait_for_36 "mig36-peer36-12345:mig-cons36-12345" "1" '.applications["dummy-sink"] | .relations.source | length'
	wait_for_36 "mig36-peer36-12345:mig-cons36-12345" "pre-mig" "$(workload_status "dummy-sink" 0).message"
	# wait for relation joined before migrate.
	# work around for fixing:
	# ERROR source prechecks failed: unit hasn't joined relation yet
	sleep 30

	# A consumed offer with no relations at all must migrate (juju/juju#23340).
	add_model_36 "mig36-peer36-12345" "mig-norel36-12345"
	${JUJU_36} switch "mig36-peer36-12345:mig-norel36-12345"
	${JUJU_36} consume "mig36-off-src-12345:admin/mig-offer36-12345.db-src" lone

	migrate_36 "mig36-peer36-12345" "mig-cons36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-peer36-12345" "mig-cons36-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-cons36-12345"

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	# The relation identity must have survived the import (juju/juju#23339):
	# there is no show-relation command, so assert via show-unit
	# relation-info that the relation still carries the pre-migration
	# application settings (the provider side token) in its related units.
	unit_out=$(juju show-unit -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-cons36-12345" dummy-sink/0 --format=json 2>&1 || true)
	token_val=$(echo "${unit_out}" | yq -r '.["dummy-sink/0"]["relation-info"][0]["related-units"] | .[] | .data["token"]' 2>/dev/null | head -n1)
	if [[ ${token_val} != "pre-mig" ]]; then
		red "Failed: expected the relation settings token to be pre-mig, got ${token_val:-<empty>}"
		echo "${unit_out}"
		exit 1
	fi
	rel_info_count=$(echo "${unit_out}" | yq '.["dummy-sink/0"]["relation-info"] | length')
	if [[ ${rel_info_count} != "1" ]]; then
		red "Failed: expected 1 relation on dummy-sink/0, got ${rel_info_count}"
		echo "${unit_out}"
		exit 1
	fi
	status_out=$(juju status --format=json 2>&1 || true)
	rel_count=$(echo "${status_out}" | yq '.applications["dummy-sink"].relations.source | length')
	if [[ ${rel_count} != "1" ]]; then
		red "Failed: expected 1 relation on dummy-sink:source, got ${rel_count}"
		echo "${unit_out}"
		exit 1
	fi

	# The offerer is still on juju 3.6: mixed-version cross-model relations
	# must keep flowing after the consumer migration.
	${JUJU_36} config -m "mig36-off-src-12345:mig-offer36-12345" dummy-source token=post-mig
	wait_for "post-mig" "$(workload_status "dummy-sink" 0).message"

	# The second consumed offer ("other") must also be usable after the
	# migration: on juju 3.6 a second CMR connection from the same consumer
	# was impossible (juju/juju#23338); the migrated 4.0 model connects it.
	juju integrate dummy-sink other
	wait_for "2" '.applications["dummy-sink"] | .relations.source | length'
	${JUJU_36} config -m "mig36-off-src-12345:mig-offer36-12345" ds2 token=ds2-token
	wait_for "ds2-token" "$(workload_status "dummy-sink" 0).message"

	# The unconsumed-offer model migrates too (juju/juju#23340, fixed).
	migrate_36 "mig36-peer36-12345" "mig-norel36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-peer36-12345" "mig-norel36-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-norel36-12345"
	check_contains "$(juju status -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-norel36-12345" --format=json | yq -r '.["application-endpoints"] | keys | .[]')" "lone"

	# The offer and its macaroon must still be usable after the migration:
	# deploy a local consumer and relate it to the migrated saas application.
	# A fresh token keeps the assertion independent of the earlier steps.
	${JUJU_36} config -m "mig36-off-src-12345:mig-offer36-12345" dummy-source token=reliving
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04
	juju integrate dummy-sink lone
	wait_for "lone" '.applications["dummy-sink"] | .relations.source[0]'
	wait_for "reliving" "$(workload_status "dummy-sink" 0).message"

	# Clean up.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "mig-cons36-12345"
	destroy_model "mig-norel36-12345"
	destroy_controller_36 "mig36-peer36-12345"
	destroy_controller_36 "mig36-off-src-12345"
}

# Migrating a simple k8s model from a juju 3.6 controller into the 4.0
# controller on the same k8s cloud.
# PostgreSQL cross-model relation migration on k8s, consuming side: the
# consumer model migrates to 4.0 while the offerer stays on 3.6. Covers
# write continuity after the move and relation-secret rotation on the
# 3.6 offerer (juju/juju#23341).
run_model_migration_36_cmr_secrets_consumer() {
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
run_model_migration_36_cmr_secrets_offerer() {
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

# AWS migration: model constraints on an imported space (juju/juju#23343),
# the integrate --via egress override (juju/juju#23344) and both sides of a
# cross-model relation.
run_model_migration_36_cmr_spaces() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-aws36-12345.log"
	ensure "mig-target-aws-12345" "${file}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-aws-src-12345"

	add_model_36 "mig36-aws-src-12345" "mig-aws-off36-12345"
	# juju 3.6 lists subnets as a map keyed by CIDR.
	cidr=$(${JUJU_36} subnets --format=json 2>/dev/null | yq -r '.subnets | keys | .[0]')
	if [[ -z ${cidr} || ${cidr} == "null" ]]; then
		red "Failed: could not discover a subnet CIDR for the aws migration test"
		${JUJU_36} subnets --format=json || true
		exit 1
	fi
	${JUJU_36} switch "mig36-aws-src-12345:mig-aws-off36-12345"
	${JUJU_36} add-space mig-space "${cidr}"
	${JUJU_36} set-model-constraints "spaces=mig-space"

	${JUJU_36} deploy juju-qa-dummy-source --base ubuntu@22.04
	wait_for_36 "mig36-aws-src-12345:mig-aws-off36-12345" "dummy-source" "$(idle_condition "dummy-source")"
	${JUJU_36} offer dummy-source:sink db-src

	add_model_36 "mig36-aws-src-12345" "mig-aws-cons36-12345"
	${JUJU_36} switch "mig36-aws-src-12345:mig-aws-cons36-12345"
	${JUJU_36} deploy juju-qa-dummy-sink --base ubuntu@22.04
	wait_for_36 "mig36-aws-src-12345:mig-aws-cons36-12345" "dummy-sink" "$(idle_condition "dummy-sink")"
	${JUJU_36} consume "mig36-aws-src-12345:admin/mig-aws-off36-12345.db-src"
	# The per-relation egress override (juju/juju#23344).
	${JUJU_36} integrate dummy-sink:source db-src --via "${cidr}"
	# The consumer's --via subnets show up as the egress-subnets of the
	# consumer unit in the offerer's view of the relation.
	attempt=0
	until [[ -n $(${JUJU_36} show-unit -m "mig36-aws-src-12345:mig-aws-off36-12345" dummy-source/0 --format=json 2>/dev/null | yq -r '.["dummy-source/0"]["relation-info"][] | select(."cross-model" == true) | .["related-units"] | .[] | .data["egress-subnets"]' || true) ]]; do
		if [[ ${attempt} -ge 60 ]]; then
			red "Failed: the --via egress-subnets never reached the offerer"
			${JUJU_36} show-unit -m "mig36-aws-src-12345:mig-aws-off36-12345" dummy-source/0 --format=json || true
			exit 1
		fi
		echo "[+] (attempt ${attempt}) polling for the --via egress-subnets on the offerer"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	# Sanity: the override applied before the migration at all.
	expected_egress=$(${JUJU_36} show-unit -m "mig36-aws-src-12345:mig-aws-off36-12345" dummy-source/0 --format=json 2>/dev/null | yq -r '.["dummy-source/0"]["relation-info"][] | select(."cross-model" == true) | .["related-units"] | .[] | .data["egress-subnets"]')
	check_contains "${expected_egress}" "${cidr}"

	${JUJU_36} config -m "mig36-aws-src-12345:mig-aws-off36-12345" dummy-source token=pre-mig
	wait_for_36 "mig36-aws-src-12345:mig-aws-cons36-12345" "pre-mig" "$(workload_status "dummy-sink" 0).message"

	# Migrate the consuming model first: the offerer stays on 3.6 for the
	# mixed-version part of the run.
	migrate_36 "mig36-aws-src-12345" "mig-aws-cons36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-aws-src-12345" "mig-aws-cons36-12345"

	${JUJU_36} config -m "mig36-aws-src-12345:mig-aws-off36-12345" dummy-source token=post-mig
	wait_for "post-mig" "$(workload_status "dummy-sink" 0).message"

	# The offerer must still see the --via subnets after the relation
	# re-joined (juju/juju#23344).
	migrated_egress=$(${JUJU_36} show-unit -m "mig36-aws-src-12345:mig-aws-off36-12345" dummy-source/0 --format=json 2>&1 || true)
	check_contains "${migrated_egress}" "${expected_egress}"

	# Now migrate the offering model.
	migrate_36 "mig36-aws-src-12345" "mig-aws-off36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-aws-src-12345" "mig-aws-off36-12345"

	# The model constraint naming the imported space must survive
	# (juju/juju#23343).
	constraints_out=$(juju model-constraints -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-aws-off36-12345" 2>&1 || true)
	check_contains "${constraints_out}" "spaces=mig-space"

	# End-to-end flow after both sides moved.
	juju config -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-aws-off36-12345" dummy-source token=yeah-boi
	wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	# The egress override must survive the offering-side migration too
	# (juju/juju#23344): the now-4.0 offerer must still see the --via
	# subnets.
	final_egress=$(juju show-unit -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-aws-off36-12345" dummy-source/0 --format=json 2>&1 || true)
	check_contains "${final_egress}" "${expected_egress}"

	# The offer must be removed before model/controller destruction will
	# work. See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju remove-offer "admin/mig-aws-off36-12345.db-src" -c "${BOOTSTRAPPED_JUJU_CTRL_NAME}" --force -y

	# Clean up.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "mig-aws-cons36-12345"
	destroy_model "mig-aws-off36-12345"
	destroy_controller_36 "mig36-aws-src-12345"
}

# A wrench-forced failure after the target import must abort cleanly: the
# target removes the partially-imported model and the 3.6 source model
# remains functional.
run_model_migration_36_abort() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-abort36-12345.log"
	ensure "mig-target-abort-12345" "${file}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-abort-src-12345"
	add_model_36 "mig36-abort-src-12345" "mig-abort36-12345"

	${JUJU_36} switch "mig36-abort-src-12345:mig-abort36-12345"
	${JUJU_36} deploy ubuntu-lite ubuntu

	wait_for_36 "mig36-abort-src-12345:mig-abort36-12345" "ubuntu" "$(idle_condition "ubuntu")"

	# Arm the migration abort wrench on the source controller. The
	# migrationmaster worker reads this file synchronously just after the
	# model has been imported into the target, so arming it before starting
	# the migration deterministically forces the abort path.
	add_clean_func "cleanup_wrench_die_in_export_36"
	arm_wrench_die_in_export

	migrate_36 "mig36-abort-src-12345" "mig-abort36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	# Wait for the abort to run to completion on the source controller. The
	# imported model never activates on the target (the wrench fires during
	# transfer), so the abort is observed via the migrationmaster logs,
	# which are emitted into the source controller's controller-model
	# stream.
	attempt=0
	until ${JUJU_36} debug-log -m "mig36-abort-src-12345:controller" --no-tail 2>/dev/null | grep -q "setting migration phase to ABORTDONE"; do
		if [[ ${attempt} -ge 30 ]]; then
			red 'Failed: migration abort did not complete'
			${JUJU_36} debug-log -m "mig36-abort-src-12345:controller" --no-tail --lines 100 || true
			exit 1
		fi
		echo "[+] (attempt ${attempt}) polling for migration abort"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	# Assert the wrench-forced failure triggered the abort path.
	abort_logs="$(${JUJU_36} debug-log -m "mig36-abort-src-12345:controller" --no-tail 2>/dev/null || true)"
	check_contains "${abort_logs}" "wrench in the transferModel works"

	# The target controller must not have the model. Model removal on
	# the target is asynchronous (RemoveMigratingModel only marks the
	# model dead; the undertaker deletes the model database in the
	# background), so poll until the model is actually reaped before
	# asserting its absence.
	wait_target_reaped "${BOOTSTRAPPED_JUJU_CTRL_NAME}" "mig-abort36-12345"
	check_not_contains "$(juju models -c "${BOOTSTRAPPED_JUJU_CTRL_NAME}")" "mig-abort36-12345"

	# The source model must still be fully functional.
	${JUJU_36} switch "mig36-abort-src-12345:mig-abort36-12345"
	${JUJU_36} add-unit ubuntu
	wait_for_36 "mig36-abort-src-12345:mig-abort36-12345" "ubuntu" "$(idle_condition "ubuntu" 1)"

	# Clean up. The model lives on the 3.6 source controller (the abort kept
	# it there), so destroying it removes the model as well.
	destroy_controller_36 "mig36-abort-src-12345"

	# Switch back to the primary controller: the suite teardown resolves the
	# tracking model via the current controller, which must not be left
	# pointing at the destroyed source controller.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
}

# Migrating a 3.6 IAAS model carrying model-level user access grants into
# the 4.0 controller, then asserting the grants survived the import. In 4.0
# the access domain was rewritten, so model user ACLs are a real migration
# risk and currently untested.
run_model_migration_36_users_permissions() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-perm36-12345.log"
	ensure "mig-target-users-12345" "${file}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-perm-src-12345"
	add_model_36 "mig36-perm-src-12345" "mig-perm36-12345"

	${JUJU_36} switch "mig36-perm-src-12345:mig-perm36-12345"
	${JUJU_36} deploy ubuntu-lite ubuntu --base ubuntu@22.04

	wait_for_36 "mig36-perm-src-12345:mig-perm36-12345" "ubuntu" "$(idle_condition "ubuntu")"

	# Create local users and grant them model-level access. The 3.6 CLI
	# syntax for add-user and grant matches 4.0 (verified in
	# tests/suites/user/manage.sh:14-23).
	${JUJU_36} add-user alice
	${JUJU_36} add-user bob
	${JUJU_36} grant alice write "mig-perm36-12345"
	${JUJU_36} grant bob read "mig-perm36-12345"

	# Capture the pre-migration model users. show-model --format=json returns
	# a top-level map keyed by model name with a "users" map whose values
	# carry an "access" field (verified in tests/suites/user/manage.sh:26-28).
	pre_users=$(${JUJU_36} show-model "mig-perm36-12345" --format=json 2>/dev/null | yq -r '."mig-perm36-12345"."users" | keys | .[]')
	check_contains "${pre_users}" "alice"
	check_contains "${pre_users}" "bob"

	# A user-scope secret grant must also survive. grant-secret in 3.6 takes
	# the secret name and the application to grant to (see
	# run_model_migration_36 and cmd/juju/secrets/grantrevoke.go).
	user_secret_uri=$(${JUJU_36} add-secret mysecret owned-by="model" --info "this is a user secret")
	user_secret_short_uri=${user_secret_uri##*:}
	${JUJU_36} grant-secret mysecret "ubuntu"
	check_contains "$(juju36_exec_output --unit "ubuntu/0" -- secret-get "${user_secret_short_uri}")" "owned-by: model"

	migrate_36 "mig36-perm-src-12345" "mig-perm36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-perm-src-12345" "mig-perm36-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-perm36-12345"

	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	# Assert the model-level access grants survived the import. The 4.0
	# show-model json output still carries a users map with the same key
	# layout as 3.6 (model name -> users -> user -> access).
	post_users=$(juju show-model "mig-perm36-12345" --format=json 2>/dev/null | yq -r '."mig-perm36-12345"."users" | keys | .[]')
	check_contains "${post_users}" "alice"
	check_contains "${post_users}" "bob"

	alice_access=$(juju show-model "mig-perm36-12345" --format=json 2>/dev/null | yq -r '."mig-perm36-12345"."users"."alice"."access"')
	bob_access=$(juju show-model "mig-perm36-12345" --format=json 2>/dev/null | yq -r '."mig-perm36-12345"."users"."bob"."access"')
	if [[ ${alice_access} != "write" ]]; then
		red "Failed: expected alice to have write access, got ${alice_access:-<empty>}"
		exit 1
	fi
	if [[ ${bob_access} != "read" ]]; then
		red "Failed: expected bob to have read access, got ${bob_access:-<empty>}"
		exit 1
	fi

	# The user-scope secret grant must still allow the application to read
	# the secret after migration.
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get "${user_secret_short_uri}")" "owned-by: model"

	# Clean up.
	destroy_controller_36 "mig36-perm-src-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "mig-perm36-12345"
}

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
# and fails fast with the abort reason.
migrate_36() {
	local src_ctrl model target_ctrl attempt start_time elapsed

	src_ctrl=${1}
	model=${2}
	target_ctrl=${3}

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

test_model_migration_36() {
	if [ -n "$(skip 'test_model_migration_36')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"lxd"|"ec2")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration test needs the lxd or ec2 provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_36"
	)
}


test_model_migration_36_cmr_offering() {
	if [ -n "$(skip 'test_model_migration_36_cmr_offering')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration CMR offering migration test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration CMR offering migration test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"lxd"|"ec2")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration CMR offering migration test needs the lxd or ec2 provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_36_cmr_offering"
	)
}


test_model_migration_36_cmr_consuming() {
	if [ -n "$(skip 'test_model_migration_36_cmr_consuming')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration CMR consuming migration test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration CMR consuming migration test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"lxd"|"ec2")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration CMR consuming migration test needs the lxd or ec2 provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_36_cmr_consuming"
	)
}


test_model_migration_36_cmr_spaces() {
	if [ -n "$(skip 'test_model_migration_36_cmr_spaces')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration CMR spaces and egress migration test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration CMR spaces and egress migration test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"ec2")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration CMR spaces and egress migration test needs the ec2 provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_36_cmr_spaces"
	)
}


test_model_migration_36_cmr_secrets_consumer() {
	if [ -n "$(skip 'test_model_migration_36_cmr_secrets_consumer')" ]; then
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

		run "run_model_migration_36_cmr_secrets_consumer"
	)
}


test_model_migration_36_cmr_secrets_offerer() {
	if [ -n "$(skip 'test_model_migration_36_cmr_secrets_offerer')" ]; then
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

		run "run_model_migration_36_cmr_secrets_offerer"
	)
}


test_model_migration_36_abort() {
	if [ -n "$(skip 'test_model_migration_36_abort')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration abort test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration abort test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"lxd"|"ec2")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration abort test needs the lxd or ec2 provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_36_abort"
	)
}

test_model_migration_36_users_permissions() {
	if [ -n "$(skip 'test_model_migration_36_users_permissions')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration users/permissions test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration users/permissions test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"lxd"|"ec2")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration users/permissions test needs the lxd or ec2 provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_36_users_permissions"
	)
}
