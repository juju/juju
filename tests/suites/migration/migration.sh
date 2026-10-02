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


run_migration_36() {
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
run_migration_36_cmr_offering() {
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
run_migration_36_cmr_consuming() {
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


# AWS migration: model constraints on an imported space (juju/juju#23343),
# the integrate --via egress override (juju/juju#23344) and both sides of a
# cross-model relation.
run_migration_36_cmr_spaces() {
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

	# wait_for drives the current model of the 4.0 client, and migrate_36
	# left the shared current controller pointing at the 3.6 source,
	# which the 4.0 client cannot drive. Switch to the migrated consumer
	# model on the target before the post-migration waits.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-aws-cons36-12345"

	${JUJU_36} config -m "mig36-aws-src-12345:mig-aws-off36-12345" dummy-source token=post-mig
	wait_for "post-mig" "$(workload_status "dummy-sink" 0).message"

	# The offerer must still see the --via subnets after the relation
	# re-joined (juju/juju#23344).
	migrated_egress=$(${JUJU_36} show-unit -m "mig36-aws-src-12345:mig-aws-off36-12345" dummy-source/0 --format=json 2>&1 || true)
	check_contains "${migrated_egress}" "${expected_egress}"

	# Now migrate the offering model.
	migrate_36 "mig36-aws-src-12345" "mig-aws-off36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-aws-src-12345" "mig-aws-off36-12345"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-aws-off36-12345"

	# The model constraint naming the imported space must survive
	# (juju/juju#23343).
	constraints_out=$(juju model-constraints -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-aws-off36-12345" 2>&1 || true)
	check_contains "${constraints_out}" "spaces=mig-space"

	# The space itself must have moved with the model: a migration that
	# drops the space but keeps the dangling constraint reference would
	# pass the check above while breaking every future constrained
	# placement (juju/juju#23343).
	spaces_out=$(juju spaces -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-aws-off36-12345" 2>&1 || true)
	check_contains "${spaces_out}" "mig-space"

	# Exercise constrained placement against the imported space: adding
	# a unit must place with the spaces=mig-space model constraint in
	# effect (and would fail to place at all if the constraint
	# referenced a space that no longer exists).
	juju add-unit dummy-source
	wait_for "dummy-source" "$(idle_condition "dummy-source" 1)"

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
run_migration_36_abort() {
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

	migrate_36 "mig36-abort-src-12345" "mig-abort36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}" "expect-abort"

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
run_migration_36_users_permissions() {
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
	# run_migration_36 and cmd/juju/secrets/grantrevoke.go).
	user_secret_uri=$(${JUJU_36} add-secret mysecret owned-by="model" --info "this is a user secret")
	user_secret_short_uri=${user_secret_uri##*:}
	${JUJU_36} grant-secret mysecret "ubuntu"
	check_contains "$(juju36_exec_output --unit "ubuntu/0" -- secret-get "${user_secret_short_uri}")" "owned-by: model"

	# User accounts are not migrated, so the users must already exist on
	# the destination controller for the 3.6 migration precheck to pass;
	# their model grants travel with the model and are asserted after the
	# import.
	juju add-user -c "${BOOTSTRAPPED_JUJU_CTRL_NAME}" alice
	juju add-user -c "${BOOTSTRAPPED_JUJU_CTRL_NAME}" bob

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

test_migration_36() {
	if [ -n "$(skip 'test_migration_36')" ]; then
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

		run "run_migration_36"
	)
}


test_migration_36_cmr_offering() {
	if [ -n "$(skip 'test_migration_36_cmr_offering')" ]; then
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

		run "run_migration_36_cmr_offering"
	)
}


test_migration_36_cmr_consuming() {
	if [ -n "$(skip 'test_migration_36_cmr_consuming')" ]; then
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

		run "run_migration_36_cmr_consuming"
	)
}


test_migration_36_cmr_spaces() {
	if [ -n "$(skip 'test_migration_36_cmr_spaces')" ]; then
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

		run "run_migration_36_cmr_spaces"
	)
}

test_migration_36_abort() {
	if [ -n "$(skip 'test_migration_36_abort')" ]; then
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

		run "run_migration_36_abort"
	)
}

test_migration_36_users_permissions() {
	if [ -n "$(skip 'test_migration_36_users_permissions')" ]; then
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

		run "run_migration_36_users_permissions"
	)
}
