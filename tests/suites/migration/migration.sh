#!/usr/bin/env -S bash -e

# Integration tests for model migrations into a controller built from this
# branch.
#
# The juju 3.6 scenarios bootstrap their source controllers with a juju 3.6
# client binary (default /snap/bin/juju_36, override with
# JUJU_MIGRATION_36_BIN); the migration target is the suite's own
# controller.
#
# The same-version scenarios migrate between two controllers built from this
# branch: the suite controller and one bootstrapped with
# bootstrap_alt_controller.

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

	# Log transfer is deliberately not asserted: 4.0 does not backfill the
	# model's pre-migration logs (juju/juju#23410), and even once fixed the
	# target log file is append-ordered, so records from the source land
	# interleaved with post-migration lines and no strict content assertion
	# can hold across the migration.
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

# Migrating a simple one-application model from one controller to another.
run_migration_basic() {
	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure we have another controller available.
	bootstrap_alt_controller "alt-model-migration"
	juju switch "alt-model-migration"
	add_model "model-migration"
	juju model-config -m controller "logging-config=#migration=DEBUG"
	juju model-config -m model-migration "logging-config=#migration=DEBUG"

	juju deploy ubuntu-lite ubuntu

	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	# create user secrets.
	user_secret_uri=$(juju --show-log add-secret mysecret owned-by="model" --info "this is a user secret")
	user_secret_short_uri=${user_secret_uri##*:}
	check_contains "$(juju --show-log show-secret mysecret --revisions | yq ".${user_secret_short_uri}.description")" 'this is a user secret'
	juju --show-log grant-secret mysecret "ubuntu"
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get $user_secret_short_uri)" "owned-by: model"

	# create charm-owned secret.
	unit_owned_secret_uri=$(juju_exec_output --unit ubuntu/0 -- secret-add --owner unit owned-by=ubuntu/0)
	unit_owned_secret_short_uri=${unit_owned_secret_uri##*:}
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get $unit_owned_secret_short_uri)" "owned-by: ubuntu/0"
	app_owned_secret_uri=$(juju_exec_output --unit ubuntu/0 -- secret-add owned-by=ubuntu)
	app_owned_secret_short_uri=${app_owned_secret_uri##*:}
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get $app_owned_secret_short_uri)" "owned-by: ubuntu"

	# Capture logs to ensure they are migrated
	old_logs="$(juju debug-log --no-tail -l DEBUG)"

	juju model-config -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:controller" "logging-config=#migration=DEBUG"
	juju migrate "model-migration" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	# Wait for the new model migration to appear in the alt controller.
	wait_for_model "model-migration"

	# Once the model has appeared, switch to it.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:model-migration"

	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	# Check that the secrets are still present and accessible.
	check_contains "$(juju --show-log show-secret mysecret --revisions | yq ".${user_secret_short_uri}.description")" 'this is a user secret'
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get $user_secret_short_uri)" "owned-by: model"
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get $unit_owned_secret_short_uri)" "owned-by: ubuntu/0"
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get $app_owned_secret_short_uri)" "owned-by: ubuntu"

	# check we can still create new secrets.
	user_secret_uri1=$(juju --show-log add-secret mysecret1 owned-by="model-as-well" --info "this is another user secret")
	user_secret_short_uri1=${user_secret_uri1##*:}
	check_contains "$(juju --show-log show-secret mysecret1 --revisions | yq ".${user_secret_short_uri1}.description")" 'this is another user secret'
	unit_owned_secret_uri1=$(juju_exec_output --unit ubuntu/0 -- secret-add --owner unit owned-by=ubuntu/0)
	unit_owned_secret_short_uri1=${unit_owned_secret_uri1##*:}
	check_contains "$(juju_exec_output --unit "ubuntu/0" -- secret-get $unit_owned_secret_short_uri1)" "owned-by: ubuntu/0"

	# Add a unit to ubuntu to ensure the model is functional
	juju add-unit ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu" 1)"

	# Clean up.
	destroy_controller "alt-model-migration"

	# Add a unit to ubuntu to ensure the model is functional
	juju add-unit ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu" 2)"

	# Assert old logs have been transfered over
	new_logs="$(juju debug-log --no-tail --replay -l DEBUG)"
	if [[ ${new_logs} != *"${old_logs}"* ]]; then
		echo "$(red 'logs failed to migrate')"
		exit 1
	fi

	destroy_model "model-migration"
}

# Migrating a model where the export fails mid-transfer (forced via wrench)
# must abort cleanly: the target removes the partially-imported model and the
# source model remains functional.
run_migration_abort() {
	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure we have another controller available.
	bootstrap_alt_controller "alt-model-migration-abort"
	juju switch "alt-model-migration-abort"
	add_model "model-migration-abort"
	juju model-config -m controller "logging-config=#migration=DEBUG"
	juju model-config -m model-migration-abort "logging-config=#migration=DEBUG"

	juju deploy ubuntu-lite ubuntu

	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	# Arm the migration abort wrench on the source controller. The
	# migrationmaster worker reads this file synchronously just after the
	# model has been imported into the target, so arming it before starting
	# the migration deterministically forces the abort path.
	add_clean_func "cleanup_wrench_die_in_export"
	add_wrench_die_in_export

	juju model-config -m "${BOOTSTRAPPED_JUJU_CTRL_NAME}:controller" "logging-config=#migration=DEBUG"
	if ! juju migrate "model-migration-abort" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"; then
		red 'Failed: juju migrate command failed'
		exit 1
	fi

	# Wait for the abort to run to completion on the source controller. The
	# imported model never activates on the target (the wrench fires during
	# IMPORT), so the abort is observed via the migrationmaster logs, which
	# are emitted into the source controller's controller-model stream.
	attempt=0
	until juju debug-log -m controller --no-tail 2>/dev/null | grep -q "setting migration phase to ABORTDONE"; do
		if [[ ${attempt} -ge 30 ]]; then
			red 'Failed: migration abort did not complete'
			exit 1
		fi
		echo "[+] (attempt ${attempt}) polling for migration abort"
		sleep "${SHORT_TIMEOUT}"
		attempt=$((attempt + 1))
	done

	# Assert the wrench-forced failure triggered the abort path and the
	# target-side import was cleaned up.
	abort_logs="$(juju debug-log -m controller --no-tail)"
	check_contains "${abort_logs}" "model data transfer failed, wrench in the transferModel works"
	check_contains "${abort_logs}" "aborted, removing model from target controller"

	# The target controller must not have the model.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	check_not_contains "$(juju models)" "model-migration-abort"

	# The source model must still be fully functional.
	juju switch "alt-model-migration-abort:model-migration-abort"
	juju add-unit ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu" 1)"

	# Clean up. The model lives on the alt controller (abort kept it on the
	# source), so destroying the alt controller removes it as well.
	destroy_controller "alt-model-migration-abort"

	# Switch back to the primary controller: the suite teardown resolves the
	# tracking model via the current controller, which must not be left
	# pointing at the destroyed alt controller.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
}

# add_wrench_die_in_export arms the "migrationmaster/die-in-export" wrench,
# which makes the migrationmaster worker fail the transfer right after the
# model is imported into the target, forcing the migration to abort.
# The wrench directory and file must be owned by the root user (the user the
# agents run as), hence the sudo.
add_wrench_die_in_export() {
	juju ssh -m controller controller/0 \
		'sudo mkdir -p /var/lib/juju/wrench && echo "die-in-export" | sudo tee /var/lib/juju/wrench/migrationmaster >/dev/null'
	juju ssh -m controller controller/0 \
		'sudo test -f /var/lib/juju/wrench/migrationmaster && sudo grep -x "die-in-export" /var/lib/juju/wrench/migrationmaster >/dev/null' || exit 1
}

cleanup_wrench_die_in_export() {
	juju ssh -m controller controller/0 \
		'sudo mkdir -p /var/lib/juju/wrench && : | sudo tee /var/lib/juju/wrench/migrationmaster >/dev/null' >/dev/null 2>&1 || true
}

# Migrating an active model from stable to devel controller (twice).
# Method:
#   - Bootstraps a devel controller
#   - Bootstraps the provided stable controller deploys an active application
#   - Migrates from stable -> devel controller
#   - Asserts the deployed application continues to work
run_migration_version() {
	# Record the current value then restore later once this run done.
	SHORT_GIT_COMMIT_VALUE="$SHORT_GIT_COMMIT"
	JUJU_VERSION_VALUE="$JUJU_VERSION"
	# Reset JUJU_VERSION and SHORT_GIT_COMMIT for stable bootstrap
	unset SHORT_GIT_COMMIT
	juju_version_without_build_number=$(echo "$JUJU_VERSION" | sed "s/.$JUJU_BUILD_NUMBER//")
	export JUJU_VERSION=$juju_version_without_build_number
	major_minor=$(echo "$JUJU_VERSION" | cut -d'-' -f1 | cut -d'.' -f1,2)

	# This test is slow sometimes to operate with the charmhub. So, we need to enlarge the timeout for wait_for.
	wait_for_timeout=1800

	# test against 3.0/stable channel for 3.0 and develop branch.
	channel="$major_minor/stable"

	stable_version=$(snap info juju | yq ".channels[\"$channel\"]" | cut -d' ' -f1)
	echo "stable_version ==> $stable_version"
	if [[ $stable_version == "--" || $stable_version == null ]]; then
		echo "==> SKIP: run_migration_version because $channel is not published yet!"
		return
	fi
	export JUJU_VERSION=$stable_version

	# Ensure we have another controller available.
	bootstrap_alt_controller "alt-model-migration-version-stable"
	juju --show-log switch "alt-model-migration-version-stable"
	add_model "model-migration-version-stable"

	juju --show-log deploy easyrsa
	juju --show-log deploy etcd
	juju --show-log integrate etcd easyrsa
	juju --show-log add-unit -n 2 etcd

	wait_for "active" '.applications["easyrsa"] | ."application-status".current' $wait_for_timeout
	wait_for "easyrsa" "$(idle_condition "easyrsa")" $wait_for_timeout
	wait_for "active" '.applications["etcd"] | ."application-status".current' $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 0)" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 1)" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 2)" $wait_for_timeout

	wait_for "active" "$(workload_status "etcd" 0).current" $wait_for_timeout
	wait_for "active" "$(workload_status "etcd" 1).current" $wait_for_timeout
	wait_for "active" "$(workload_status "etcd" 2).current" $wait_for_timeout

	juju --show-log run etcd/0 etcd/1 etcd/2 --wait=5m health

	juju --show-log migrate "model-migration-version-stable" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju --show-log switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	# Wait for the new model migration to appear in the devel controller.
	wait_for_model "model-migration-version-stable"

	# Once the model has appeared, switch to it.
	juju --show-log switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:model-migration-version-stable"

	wait_for "easyrsa" "$(idle_condition "easyrsa")" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 0)" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 1)" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 2)" $wait_for_timeout

	# Add a unit to etcd to ensure the model is functional
	juju add-unit -n 2 etcd
	wait_for "etcd" "$(idle_condition "etcd" 0)" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 1)" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 2)" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 3)" $wait_for_timeout
	wait_for "etcd" "$(idle_condition "etcd" 4)" $wait_for_timeout

	wait_for "active" "$(workload_status "etcd" 0).current" $wait_for_timeout
	wait_for "active" "$(workload_status "etcd" 1).current" $wait_for_timeout
	wait_for "active" "$(workload_status "etcd" 2).current" $wait_for_timeout
	wait_for "active" "$(workload_status "etcd" 3).current" $wait_for_timeout
	wait_for "active" "$(workload_status "etcd" 4).current" $wait_for_timeout

	juju --show-log run etcd/0 etcd/1 etcd/2 etcd/3 etcd/4 --wait=10m health

	# Clean up.
	destroy_controller "alt-model-migration-version-stable"

	destroy_model "model-migration-version-stable"

	# Restore these two environment variables for the rest of the tests.
	export SHORT_GIT_COMMIT="$SHORT_GIT_COMMIT_VALUE"
	export JUJU_VERSION="$JUJU_VERSION_VALUE"
}

# Migrating a model that is the offerer of a cross-model relation
# consumed by another model on the same controller.
run_migration_saas_common() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-model-migration-saas-common.log"
	ensure "model-migration-saas" "${file}"

	# Ensure we have another controller available.
	bootstrap_alt_controller "alt-model-migration-saas"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	add_model blog
	juju switch blog
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju --show-log consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
	juju --show-log relate dummy-sink dummy-source
	# wait for relation joined before migrate.
	# work around for fixing:
	# ERROR source prechecks failed: unit dummy-source/0 hasn't joined relation "dummy-source:sink remote-abaa4396b3ae409981ad83d1d04af21f:source" yet
	wait_for "dummy-source" '.applications["dummy-sink"] | .relations.source[0]'
	sleep 30

	juju switch "model-migration-saas"
	wait_for "1" '.offers["dummy-source"]["active-connected-count"]'

	juju --show-log migrate "model-migration-saas" "alt-model-migration-saas"
	sleep 5
	juju switch "alt-model-migration-saas"

	# Wait for the new model migration to appear in the alt controller.
	wait_for_model "model-migration-saas"

	# Once the model has appeared, switch to it.
	juju switch "alt-model-migration-saas:model-migration-saas"

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	# Change the dummy-source config for "token" and check that the change
	# is represented in the consuming model's dummy-sink unit.
	juju config dummy-source token=yeah-boi
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:blog"

	wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	# The offer must be removed before model/controller destruction will work.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju switch "alt-model-migration-saas:model-migration-saas"
	juju remove-offer "admin/model-migration-saas.dummy-source" --force -y

	# Clean up.
	destroy_controller "alt-model-migration-saas"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "model-migration-saas"
	destroy_model "blog"
}

# Migrating a model that is the offerer of a cross-model
# relation, consumed by a model on another controller.
run_migration_saas_external() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-model-migration-saas-external.log"
	ensure "model-migration-saas" "${file}"

	# Ensure we have controllers for the consuming model
	# and the migration target.
	bootstrap_alt_controller "model-migration-saas-consume"
	bootstrap_alt_controller "model-migration-saas-target"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	juju switch "model-migration-saas-consume"
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju --show-log consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
	juju --show-log relate dummy-sink dummy-source
	# wait for relation joined before migrate.
	wait_for "dummy-source" '.applications["dummy-sink"] | .relations.source[0]'
	sleep 30

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_for "1" '.offers["dummy-source"]["active-connected-count"]'

	juju --show-log migrate "model-migration-saas" "model-migration-saas-target"
	sleep 5
	juju switch "model-migration-saas-target"

	# Wait for the new model migration to appear in the target controller.
	wait_for_model "model-migration-saas"

	# Once the model has appeared, switch to it.
	juju switch "model-migration-saas"

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	# Change the dummy-source config for "token" and check that the change
	# is represented in the consuming model's dummy-sink unit.
	juju config dummy-source token=yeah-boi
	juju switch "model-migration-saas-consume"

	wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	# The offer must be removed before model/controller destruction will work.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju switch "model-migration-saas-target:model-migration-saas"
	juju remove-offer "admin/model-migration-saas.dummy-source" --force -y

	# Clean up.
	destroy_controller "model-migration-saas-consume"
	destroy_controller "model-migration-saas-target"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "model-migration-saas"
}

# Migrating a model that is the consumer of a cross-model
# relation, offered by a model on another controller.
run_migration_saas_consumer() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-model-migration-saas-consumer.log"
	ensure "model-migration-saas" "${file}"

	# Ensure we have controllers for the consuming model
	# and the migration target.
	bootstrap_alt_controller "model-migration-saas-consume"
	bootstrap_alt_controller "model-migration-saas-target"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	juju switch "model-migration-saas-consume"
	add_model "model-migration-consumer"
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju --show-log consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
	juju --show-log relate dummy-sink dummy-source
	# wait for relation joined before migrate.
	wait_for "dummy-source" '.applications["dummy-sink"] | .relations.source[0]'
	sleep 30

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju config dummy-source token=wait-for-it
	juju switch "model-migration-saas-consume"
	wait_for "wait-for-it" "$(workload_status "dummy-sink" 0).message"

	juju --show-log migrate "model-migration-consumer" "model-migration-saas-target"
	sleep 5
	juju switch "model-migration-saas-target"

	# Wait for the new model migration to appear in the target controller.
	wait_for_model "model-migration-consumer"

	# Once the model has appeared, switch to it.
	juju switch "model-migration-consumer"

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	# Change the dummy-source config for "token" and check that the change
	# is represented in the consuming model's dummy-sink unit.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju config dummy-source token=yeah-boi
	juju switch "model-migration-saas-target"

	wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	# The offer must be removed before model/controller destruction will work.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju remove-offer "admin/model-migration-saas.dummy-source" --force -y

	# Clean up.
	destroy_controller "model-migration-saas-consume"
	destroy_controller "model-migration-saas-target"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "model-migration-saas"
}

test_migration_basic() {
	if [ -n "$(skip 'test_migration_basic')" ]; then
		echo "==> SKIP: Asked to skip model migration tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_migration_basic"
	)
}

test_migration_abort() {
	if [ -n "$(skip 'test_migration_abort')" ]; then
		echo "==> SKIP: Asked to skip model migration abort test"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_migration_abort"
	)
}

test_migration_version() {
	if [ -n "$(skip 'test_migration_version')" ]; then
		echo "==> SKIP: Asked to skip model migration version tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_migration_version"
	)
}

test_migration_saas_common() {
	if [ -n "$(skip 'test_migration_saas_common')" ]; then
		echo "==> SKIP: Asked to skip model migration saas common tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_migration_saas_common"
	)
}

test_migration_saas_external() {
	if [ -n "$(skip 'test_migration_saas_external')" ]; then
		echo "==> SKIP: Asked to skip model migration saas external tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_migration_saas_external"
	)
}

test_migration_saas_consumer() {
	if [ -n "$(skip 'test_migration_saas_consumer')" ]; then
		echo "==> SKIP: Asked to skip model migration saas consumer tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_migration_saas_consumer"
	)
}

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

# Migrating a consuming model with a per-relation egress override (juju
# integrate --via CIDRs) from a juju 3.6 source controller into the suite's
# 4.0 target controller must keep the override: the migration import has to
# persist the admin-supplied per-relation CIDRs, and network-get in relation
# scope must report them instead of the egress-subnets model default. This
# is the end-to-end counterpart of
# domain/modelmigration/import_relation_network_test.go
# (TestRelationEgressOverride).
#
# Only the supported 3.6 -> 4.0 migration direction is exercised; the
# offerer model stays on the 3.6 source controller, so the migrated
# relation is also a mixed-version cross-model relation. A 4.0 -> 4.0
# migration would need the MigrationTarget v8 / SerializedModelV2 path
# that lands in 4.1.
#
# The migration import runs on the 4.0 target controller's jujud, so the
# target must run the agent binaries built from this checkout
# (BUILD_AGENT=true for local suite runs). With the released agents the
# import silently runs stale code and drops the egress override: the very
# bug this test guards against.
run_migration_36_relation_egress_override() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-mig-egress36-12345.log"
	ensure "mig-target-egress-12345" "${file}"

	# A model-level egress default (egress-subnets) that differs from the
	# per-relation override, so that a dropped override is distinguishable
	# from the model default after the migration.
	egress_default="203.0.113.0/24"

	# The per-relation egress override applied with juju integrate --via.
	via_cidrs="198.51.100.0/25,192.0.2.0/24"
	via1="${via_cidrs%%,*}"
	via2="${via_cidrs##*,}"

	add_clean_func "cleanup_mig_controllers_36"
	bootstrap_controller_36 "mig36-egress-src-12345"

	add_model_36 "mig36-egress-src-12345" "mig-egress-off36-12345"
	${JUJU_36} switch "mig36-egress-src-12345:mig-egress-off36-12345"
	${JUJU_36} deploy juju-qa-dummy-source --base ubuntu@22.04
	${JUJU_36} config dummy-source token=yeah-boi
	wait_for_36 "mig36-egress-src-12345:mig-egress-off36-12345" "dummy-source" "$(idle_condition "dummy-source")"
	${JUJU_36} offer dummy-source:sink db-src

	add_model_36 "mig36-egress-src-12345" "mig-egress-cons36-12345"
	${JUJU_36} switch "mig36-egress-src-12345:mig-egress-cons36-12345"
	${JUJU_36} model-config "egress-subnets=${egress_default}"
	${JUJU_36} deploy juju-qa-dummy-sink --base ubuntu@22.04

	# Same-controller consume: the offer creates the remote application
	# and remote entity tokens that the 4.0 import must map the egress
	# CIDRs onto. 3.6 exports local offers with the source controller's
	# connection info (state/migrations/externalcontrollers.go), so the
	# migrated model can reach back to the source controller to keep the
	# relation alive.
	${JUJU_36} consume "mig36-egress-src-12345:admin/mig-egress-off36-12345.db-src"

	# The per-relation egress override under test.
	${JUJU_36} integrate dummy-sink:source db-src --via "${via_cidrs}"

	wait_for_36 "mig36-egress-src-12345:mig-egress-cons36-12345" "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	# Check the per-relation egress override before migrating. The
	# per-relation egress policy (--via) is only reported when network-get
	# runs in relation scope (-r); without it the hook tool returns
	# binding-level egress, which is always the egress-subnets model
	# default. The `--` separator keeps juju exec from consuming the hook
	# tool's flags.
	echo "Model default (expect ${egress_default}):"
	OUT="$(${JUJU_36} model-config egress-subnets)"
	echo "${OUT}"
	check_contains "${OUT}" "${egress_default}"

	echo "Unit view of the relation (expect ${via_cidrs}):"
	unit_view="$(juju36_exec_output --unit dummy-sink/0 -- network-get source -r 0 --format=yaml || true)"
	echo "${unit_view}"

	# Each --via CIDR must be reported by the relation-scoped egress
	# list, and the list must hold exactly the two of them.
	echo "${unit_view}" | yq -e ".egress-subnets[] | select(. == \"${via1}\")" >/dev/null || { red "Failed: expected ${via1} among the relation egress subnets"; exit 1; }
	echo "${unit_view}" | yq -e ".egress-subnets[] | select(. == \"${via2}\")" >/dev/null || { red "Failed: expected ${via2} among the relation egress subnets"; exit 1; }
	echo "${unit_view}" | yq -e '.egress-subnets | length == 2' >/dev/null || { red "Failed: expected exactly 2 relation egress subnets"; exit 1; }

	# wait for relation joined before migrate.
	# work around for fixing:
	# ERROR source prechecks failed: unit hasn't joined relation yet
	sleep 30

	migrate_36 "mig36-egress-src-12345" "mig-egress-cons36-12345" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_source_reaped_36 "mig36-egress-src-12345" "mig-egress-cons36-12345"

	# wait_for drives the current model of the 4.0 client, and migrate_36
	# left the shared current controller pointing at the 3.6 source,
	# which the 4.0 client cannot drive. Switch to the migrated consumer
	# model on the target before the post-migration waits.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:mig-egress-cons36-12345"

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	# Check the per-relation egress override survived the migration.
	echo "Model default (expect ${egress_default}):"
	OUT="$(juju model-config egress-subnets)"
	echo "${OUT}"
	check_contains "${OUT}" "${egress_default}"

	echo "Unit view of the relation (expect ${via_cidrs}):"
	unit_view="$(juju_exec_output --unit dummy-sink/0 -- network-get source -r 0 --format=yaml || true)"
	echo "${unit_view}"

	echo "${unit_view}" | yq -e ".egress-subnets[] | select(. == \"${via1}\")" >/dev/null || { red "Failed: expected ${via1} among the relation egress subnets"; exit 1; }
	echo "${unit_view}" | yq -e ".egress-subnets[] | select(. == \"${via2}\")" >/dev/null || { red "Failed: expected ${via2} among the relation egress subnets"; exit 1; }
	echo "${unit_view}" | yq -e '.egress-subnets | length == 2' >/dev/null || { red "Failed: expected exactly 2 relation egress subnets"; exit 1; }

	# Clean up. The consumer model is destroyed on the target while the
	# source controller is still alive, which breaks the cross-model
	# relation cleanly; destroy_controller_36 removes the offerer's offer
	# before destroying the source controller.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "mig-egress-cons36-12345"
	destroy_controller_36 "mig36-egress-src-12345"
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

test_migration_36_relation_egress_override() {
	if [ -n "$(skip 'test_migration_36_relation_egress_override')" ]; then
		echo "==> SKIP: Asked to skip juju 3.6 model migration relation egress override test"
		return
	fi

	if ! gate_reason=$(mig36_gate 2>&1); then
		echo "==> SKIP: juju 3.6 model migration relation egress override test: ${gate_reason}"
		return
	fi

	case "${BOOTSTRAP_PROVIDER}" in
	"lxd"|"ec2")
		;;
	*)
		echo "==> SKIP: juju 3.6 model migration relation egress override test needs the lxd or ec2 provider"
		return
		;;
	esac

	(
		set_verbosity

		cd .. || exit

		run "run_migration_36_relation_egress_override"
	)
}
