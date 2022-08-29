# Migrating a simple one-application model from one controller to another.
run_model_migration() {
	# Echo out to ensure nice output to the test suite.
	echo

	# Ensure we have another controller available.
	bootstrap_alt_controller "alt-model-migration"
	juju switch "alt-model-migration"
	juju add-model "model-migration"

	juju deploy ubuntu

	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	# Capture logs to ensure they are migrated
	old_logs="$(juju debug-log --no-tail -l DEBUG)"

	juju migrate "model-migration" "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"

	# Wait for the new model migration to appear in the alt controller.
	wait_for_model "model-migration"

	# Once the model has appeared, switch to it.
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:model-migration"

	wait_for "ubuntu" "$(idle_condition "ubuntu")"

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
	# Reset JUJU_VERSION and SHORT_GIT_COMMIT for stable bootstrap
	unset SHORT_GIT_COMMIT
	juju_version_without_build_number=$(echo "$JUJU_VERSION" | sed "s/.$JUJU_BUILD_NUMBER//")
	export JUJU_VERSION=$juju_version_without_build_number
	major_minor=$(echo "$JUJU_VERSION" | cut -d'-' -f1 | cut -d'.' -f1,2)

	# test against beta channel for devel
	# TODO: change back to stable once 3.0 released.
	# channel="$major_minor/beta"  # 3.0
	channel="$major_minor/stable" # 2.9

	stable_version=$(snap info juju | yq ".channels[\"$channel\"]" | cut -d' ' -f1)
	echo "stable_version ==> $stable_version"
	if [[ $stable_version == "--" || $stable_version == null ]]; then
		echo "==> SKIP: run_model_migration_version because $channel is not published yet!"
		exit 0
	fi
	export JUJU_VERSION=$stable_version

	# Unset to re-generate from the new agent-version.
	unset BOOTSTRAP_ADDITIONAL_ARGS
	# Ensure we have another controller available.
	bootstrap_alt_controller "alt-model-migration-version-stable"
	juju --show-log switch "alt-model-migration-version-stable"
	juju --show-log add-model "model-migration-version-stable"

	juju --show-log deploy easyrsa
	juju --show-log deploy etcd
	juju --show-log add-relation etcd easyrsa
	juju --show-log add-unit -n 2 etcd

	wait_for "active" '.applications["easyrsa"] | ."application-status".current'
	wait_for "easyrsa" "$(idle_condition "easyrsa" 0)"
	wait_for "active" '.applications["etcd"] | ."application-status".current'
	wait_for "etcd" "$(idle_condition "etcd" 1 0)"
	wait_for "etcd" "$(idle_condition "etcd" 1 1)"
	wait_for "etcd" "$(idle_condition "etcd" 1 2)"

	wait_for "active" "$(workload_status "etcd" 0).current"
	wait_for "active" "$(workload_status "etcd" 1).current"
	wait_for "active" "$(workload_status "etcd" 2).current"

	# juju --show-log run etcd/0 etcd/1 etcd/2 --wait=5m health  # 3.0
	juju --show-log run-action etcd/0 etcd/1 etcd/2 --wait=5m health # 2.9

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

	# juju --show-log run etcd/0 etcd/1 etcd/2 etcd/3 etcd/4 --wait=50m health  # 3.0
	juju --show-log run-action etcd/0 etcd/1 etcd/2 etcd/3 etcd/4 --wait=50m health # 2.9

	# Clean up.
	destroy_controller "alt-model-migration-version-stable"

	destroy_model "model-migration-version-stable"
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
	juju deploy juju-qa-dummy-source
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	juju add-model blog
	juju switch blog
	juju deploy juju-qa-dummy-sink

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
	juju relate dummy-sink dummy-source

	juju switch "model-migration-saas"
	wait_for "1" '.offers["dummy-source"]["active-connected-count"]'

	juju migrate "model-migration-saas" "alt-model-migration-saas"
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
	juju deploy juju-qa-dummy-source
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	juju switch "model-migration-saas-consume"
	juju deploy juju-qa-dummy-sink

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
	juju relate dummy-sink dummy-source

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	wait_for "1" '.offers["dummy-source"]["active-connected-count"]'

	juju migrate "model-migration-saas" "model-migration-saas-target"
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
	juju deploy juju-qa-dummy-source
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	juju switch "model-migration-saas-consume"
	juju add-model "model-migration-consumer"
	juju deploy juju-qa-dummy-sink

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-migration-saas.dummy-source"
	juju relate dummy-sink dummy-source

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	juju config dummy-source token=wait-for-it
	juju switch "model-migration-saas-consume"
	wait_for "wait-for-it" "$(workload_status "dummy-sink" 0).message"

	juju migrate "model-migration-consumer" "model-migration-saas-target"
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
		run "run_model_migration_version"
		run "run_model_migration_saas_common"
		run "run_model_migration_saas_external"
		run "run_model_migration_saas_consumer"
	)
}

bootstrap_alt_controller() {
	local name

	name=${1}

	START_TIME=$(date +%s)
	echo "====> Bootstrapping ${name}"

	file="${TEST_DIR}/${name}.log"
	juju_bootstrap "${BOOTSTRAP_CLOUD}" "${name}" "misc" "${file}"

	END_TIME=$(date +%s)
	echo "====> Bootstrapped ${name} ($((END_TIME - START_TIME))s)"
}
