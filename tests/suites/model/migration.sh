# Migrating a simple one-application model from one controller to another.
run_model_migration() {
	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure we have another controller available.
	bootstrap_alt_controller "alt-model-migration"
	juju switch "alt-model-migration"
	add_model "model-migration"
	juju model-config -m controller "logging-config=#migration=DEBUG"
	juju model-config -m model-migration "logging-config=#migration=DEBUG"

	juju deploy jameinel-ubuntu-lite ubuntu

	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	# create user secrets.
	user_secret_uri=$(juju --show-log add-secret mysecret owned-by="model" --info "this is a user secret")
	user_secret_short_uri=${user_secret_uri##*:}
	check_contains "$(juju --show-log show-secret mysecret --revisions | yq ".${user_secret_short_uri}.description")" 'this is a user secret'
	juju --show-log grant-secret mysecret "ubuntu"
	check_contains "$(juju exec --unit "ubuntu/0" -- secret-get $user_secret_short_uri)" "owned-by: model"

	# create charm-owned secret.
	unit_owned_secret_uri=$(juju exec --unit ubuntu/0 -- secret-add --owner unit owned-by=ubuntu/0)
	unit_owned_secret_short_uri=${unit_owned_secret_uri##*:}
	check_contains "$(juju exec --unit "ubuntu/0" -- secret-get $unit_owned_secret_short_uri)" "owned-by: ubuntu/0"
	app_owned_secret_uri=$(juju exec --unit ubuntu/0 -- secret-add owned-by=ubuntu)
	app_owned_secret_short_uri=${app_owned_secret_uri##*:}
	check_contains "$(juju exec --unit "ubuntu/0" -- secret-get $app_owned_secret_short_uri)" "owned-by: ubuntu"

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
	check_contains "$(juju exec --unit "ubuntu/0" -- secret-get $user_secret_short_uri)" "owned-by: model"
	check_contains "$(juju exec --unit "ubuntu/0" -- secret-get $unit_owned_secret_short_uri)" "owned-by: ubuntu/0"
	check_contains "$(juju exec --unit "ubuntu/0" -- secret-get $app_owned_secret_short_uri)" "owned-by: ubuntu"

	# check we can still create new secrets.
	user_secret_uri1=$(juju --show-log add-secret mysecret1 owned-by="model-as-well" --info "this is another user secret")
	user_secret_short_uri1=${user_secret_uri1##*:}
	check_contains "$(juju --show-log show-secret mysecret1 --revisions | yq ".${user_secret_short_uri1}.description")" 'this is another user secret'
	unit_owned_secret_uri1=$(juju exec --unit ubuntu/0 -- secret-add --owner unit owned-by=ubuntu/0)
	unit_owned_secret_short_uri1=${unit_owned_secret_uri1##*:}
	check_contains "$(juju exec --unit "ubuntu/0" -- secret-get $unit_owned_secret_short_uri1)" "owned-by: ubuntu/0"

	# Add a unit to ubuntu to ensure the model is functional
	juju add-unit ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu" 0 1)"

	# Clean up.
	destroy_controller "alt-model-migration"

	# Add a unit to ubuntu to ensure the model is functional
	juju add-unit ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu" 0 2)"

	# Assert old logs have been transfered over
	new_logs="$(juju debug-log --no-tail --replay -l DEBUG)"
	if [[ ${new_logs} != *"${old_logs}"* ]]; then
		echo "$(red 'logs failed to migrate')"
		exit 1
	fi

	destroy_model "model-migration"
}

# Migrating an active model from stable to devel controller (twice).
# Method:
#   - Bootstraps a devel controller
#   - Bootstraps the provided stable controller deploys an active application
#   - Migrates from stable -> devel controller
#   - Asserts the deployed application continues to work
run_model_migration_version() {
	# Record the current value then restore later once this run done.
	SHORT_GIT_COMMIT_VALUE="$SHORT_GIT_COMMIT"
	JUJU_VERSION_VALUE="$JUJU_VERSION"
	# Reset JUJU_VERSION and SHORT_GIT_COMMIT for stable bootstrap
	unset SHORT_GIT_COMMIT
	juju_version_without_build_number=$(echo "$JUJU_VERSION" | sed "s/.$JUJU_BUILD_NUMBER//")
	export JUJU_VERSION=$juju_version_without_build_number
	major_minor=$(echo "$JUJU_VERSION" | cut -d'-' -f1 | cut -d'.' -f1,2)

	# test against 3.0/stable channel for 3.0 and develop branch.
	channel="$major_minor/stable"

	stable_version=$(snap info juju | yq ".channels[\"$channel\"]" | cut -d' ' -f1)
	echo "stable_version ==> $stable_version"
	if [[ $stable_version == "--" || $stable_version == null ]]; then
		echo "==> SKIP: run_model_migration_version because $channel is not published yet!"
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

	wait_for "active" '.applications["easyrsa"] | ."application-status".current'
	wait_for "easyrsa" "$(idle_condition "easyrsa" 0)"
	wait_for "active" '.applications["etcd"] | ."application-status".current' 900
	wait_for "etcd" "$(idle_condition "etcd" 1 0)"
	wait_for "etcd" "$(idle_condition "etcd" 1 1)"
	wait_for "etcd" "$(idle_condition "etcd" 1 2)"

	wait_for "active" "$(workload_status "etcd" 0).current"
	wait_for "active" "$(workload_status "etcd" 1).current"
	wait_for "active" "$(workload_status "etcd" 2).current"

	juju --show-log run etcd/0 etcd/1 etcd/2 --wait=5m health

	juju --show-log migrate "model-migration-version-stable" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju --show-log switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	# Wait for the new model migration to appear in the devel controller.
	wait_for_model "model-migration-version-stable"

	# Once the model has appeared, switch to it.
	juju --show-log switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:model-migration-version-stable"

	wait_for "easyrsa" "$(idle_condition "easyrsa" 0)"
	wait_for "etcd" "$(idle_condition "etcd" 1 0)"
	wait_for "etcd" "$(idle_condition "etcd" 1 1)"
	wait_for "etcd" "$(idle_condition "etcd" 1 2)"

	# Add a unit to etcd to ensure the model is functional
	juju add-unit -n 2 etcd
	wait_for "etcd" "$(idle_condition "etcd" 1 0)"
	wait_for "etcd" "$(idle_condition "etcd" 1 1)"
	wait_for "etcd" "$(idle_condition "etcd" 1 2)"
	wait_for "etcd" "$(idle_condition "etcd" 1 3)"
	wait_for "etcd" "$(idle_condition "etcd" 1 4)"

	wait_for "active" "$(workload_status "etcd" 0).current"
	wait_for "active" "$(workload_status "etcd" 1).current"
	wait_for "active" "$(workload_status "etcd" 2).current"
	wait_for "active" "$(workload_status "etcd" 3).current"
	wait_for "active" "$(workload_status "etcd" 4).current"

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
run_model_migration_saas_common() {
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
run_model_migration_saas_external() {
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
run_model_migration_saas_consumer() {
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

test_model_migration() {
	if [ -n "$(skip 'test_model_migration')" ]; then
		echo "==> SKIP: Asked to skip model migration tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration"
	)
}

test_model_migration_version() {
	if [ -n "$(skip 'test_model_migration_version')" ]; then
		echo "==> SKIP: Asked to skip model migration version tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_version"
	)
}

test_model_migration_saas_common() {
	if [ -n "$(skip 'test_model_migration_saas_common')" ]; then
		echo "==> SKIP: Asked to skip model migration saas common tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_saas_common"
	)
}

test_model_migration_saas_external() {
	if [ -n "$(skip 'test_model_migration_saas_external')" ]; then
		echo "==> SKIP: Asked to skip model migration saas external tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_model_migration_saas_external"
	)
}
