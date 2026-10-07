run_prometheus() {
	echo

	MODEL_NAME="test-prometheus"
	file="${TEST_DIR}/${MODEL_NAME}.log"
	bootstrap "${MODEL_NAME}" "${file}"

	juju offer controller.controller:metrics-endpoint

	juju deploy prometheus-k8s --channel 1/stable --trust
	juju relate prometheus-k8s controller.controller
	wait_for "prometheus-k8s" "$(active_idle_condition "prometheus-k8s" 0)"
	retry 'check_prometheus_targets prometheus-k8s 0' 30

	juju remove-relation prometheus-k8s controller
	# Check Juju controller is removed from Prometheus targets
	retry 'check_prometheus_no_target prometheus-k8s 0' 30
	# Check no errors in controller charm or Prometheus
	retry 'check_controller_charm_active' 10
	retry 'check_app_active prometheus-k8s' 10

	juju remove-application prometheus-k8s --destroy-storage --no-prompt \
		--force --no-wait # TODO: remove these flags once storage bug is fixed
	destroy_controller "${MODEL_NAME}"
}

# Check the controller charm can handle multiple Prometheus relations.
run_prometheus_multiple_units() {
	echo

	MODEL_NAME="test-prometheus-multi"
	file="${TEST_DIR}/${MODEL_NAME}.log"
	bootstrap "${MODEL_NAME}" "${file}"

	juju offer controller.controller:metrics-endpoint

	juju deploy prometheus-k8s --channel 1/stable p1 --trust
	juju relate p1 controller.controller
	wait_for "p1" "$(active_idle_condition "p1" 0)"
	retry 'check_prometheus_targets p1 0' 30

	juju deploy prometheus-k8s --channel 1/stable p2 --trust
	juju relate p2 controller.controller
	wait_for "p2" "$(active_idle_condition "p2" 0)"
	retry 'check_prometheus_targets p2 0' 30

	juju add-unit p1
	wait_for "p1" "$(active_idle_condition "p1" 1)"
	retry 'check_prometheus_targets p1 1' 30

	juju remove-unit p1 --num-units 1
	# Wait until the application p1 settles before health checks
	wait_for "p1" "$(active_condition "p1" 0)"

	# Check all applications are still healthy
	retry 'check_controller_charm_active' 10
	retry 'check_app_active p1 0' 10

	juju remove-relation p2 controller
	# Wait until the application p2 settles before health checks
	wait_for "p2" "$(active_condition "p2" 1)"

	# Check Juju controller is removed from Prometheus targets
	retry 'check_prometheus_no_target p2 0' 30
	# Check no errors in controller charm or Prometheus
	retry 'check_controller_charm_active' 10
	retry 'check_app_active p2 1' 10

	juju remove-relation p1 controller

	# Check Juju controller is removed from Prometheus targets
	retry 'check_prometheus_no_target p1 0' 30
	# Check no errors in controller charm or Prometheus
	retry 'check_controller_charm_active' 10
	# Ensure p1 is still healty
	wait_for "p1" "$(active_condition "p1" 0)"

	juju remove-application p1 --destroy-storage --no-prompt \
		--force --no-wait # TODO: remove these flags once storage bug is fixed
	juju remove-application p2 --destroy-storage --no-prompt \
		--force --no-wait # TODO: remove these flags once storage bug is fixed
	destroy_controller "${MODEL_NAME}"
}

run_prometheus_cross_controller() {
	echo

	CONTROLLER_MODEL_NAME="test-prometheus-cmr-ctrlr"
	file="${TEST_DIR}/${CONTROLLER_MODEL_NAME}.log"
	bootstrap "${CONTROLLER_MODEL_NAME}" "${file}"
	CONTROLLER_NAME=$(juju controllers --format json | yq -r '."current-controller"')

	# Prometheus must be deployed on k8s. By default, we choose microk8s, but you
	# can set the K8S_CLOUD environment variable to select a different cluster.
	K8S_CLOUD=${K8S_CLOUD:-microk8s}
	PROMETHEUS_MODEL_NAME="test-prometheus-cmr-prom"
	file="${TEST_DIR}/${PROMETHEUS_MODEL_NAME}.log"
	BOOTSTRAP_PROVIDER='k8s' BOOTSTRAP_CLOUD="${K8S_CLOUD}" bootstrap "${PROMETHEUS_MODEL_NAME}" "${file}"

	juju offer -c "${CONTROLLER_NAME}" controller.controller:metrics-endpoint

	juju deploy prometheus-k8s --channel 1/stable --trust
	juju relate prometheus-k8s "${CONTROLLER_NAME}:controller.controller"
	wait_for "prometheus-k8s" "$(active_idle_condition "prometheus-k8s" 0)"
	retry 'check_prometheus_targets prometheus-k8s 0' 30

	juju remove-relation prometheus-k8s controller
	# Check Juju controller is removed from Prometheus targets
	retry 'check_prometheus_no_target prometheus-k8s 0' 30
	# Check no errors in controller charm or Prometheus
	retry 'check_controller_charm_active' 10
	retry 'check_app_active prometheus-k8s' 10

	juju remove-application prometheus-k8s --destroy-storage --no-prompt \
		--force --no-wait # TODO: remove these flags once storage bug is fixed
	destroy_controller "${PROMETHEUS_MODEL_NAME}"
}

# Check the Juju controller in the list of Prometheus targets.
#   usage: check_prometheus_targets <app-name> <unit-number>
check_prometheus_targets() {
	set -uo pipefail
	local app_name=$1
	local unit_number=$2

	TARGET=$(get_juju_target "$app_name" "$unit_number") || return $?
	if [[ -z $TARGET ]]; then
		echo "Juju controller not found in Prometheus targets"
		return 1
	fi

	TARGET_STATUS=$(echo $TARGET | yq -r '.health')
	if [[ $TARGET_STATUS != "up" ]]; then
		echo "Controller metrics endpoint status: $TARGET_STATUS: $(echo $TARGET | yq -r '.lastError')"
		return 1
	fi

	echo "Controller metrics endpoint is up"
}

# Check the Juju controller is not present in the list of Prometheus targets.
#   usage: check_prometheus_no_target <app-name> <unit-number>
check_prometheus_no_target() {
	set -uo pipefail
	local app_name=$1
	local unit_number=$2

	TARGET=$(get_juju_target "$app_name" "$unit_number") || return $?
	if [[ -n $TARGET ]]; then
		echo "Whoops: Juju controller still found in Prometheus targets"
		return 1
	fi

	echo "Success: Juju controller removed from Prometheus targets"
}

# Check the controller charm is healthy in the controller model. Relation
# teardown hooks may still be settling, so callers should use retry.
#   usage: check_controller_charm_active
check_controller_charm_active() {
	set -uo pipefail
	juju status -m controller --format json |
		yq -r "$(active_condition "controller")" | check "controller"
}

# Check the given application is healthy in the current model. Relation
# teardown hooks may still be settling, so callers should use retry.
#   usage: check_app_active <app-name> [app-index]
check_app_active() {
	set -uo pipefail
	local app_name=$1
	local app_index=${2:-0}
	juju status --format json |
		yq -r "$(active_condition "$app_name" "$app_index")" | check "$app_name"
}

# Extract the Juju controller from the list of Prometheus targets. Returns 2
# when the Prometheus API cannot be reached, or its pod address cannot be
# resolved, so callers can tell an unreachable API apart from an absent
# target.
#   usage: get_juju_target <app-name> <unit-number>
get_juju_target() {
	set -uo pipefail
	local app_name=$1
	local unit_number=$2

	PROM_IP=$(juju status --format json |
		yq -r ".applications.\"$app_name\".units.\"$app_name/$unit_number\".address")
	# Fail fast when the address itself cannot be resolved, so the retry
	# log states which leg failed instead of blaming the API.
	if [[ -z ${PROM_IP} || ${PROM_IP} == "null" ]]; then
		echo "could not resolve $app_name/$unit_number address" >&2
		return 2
	fi
	# Single-shot probe: the callers' retry loops provide the endurance,
	# nesting curl retries here would multiply the worst-case latency.
	if ! RESPONSE=$(curl -sS -m 10 "http://${PROM_IP}:9090/api/v1/targets"); then
		echo "Prometheus API at ${PROM_IP}:9090 unreachable" >&2
		return 2
	fi
	echo "$RESPONSE" |
		yq '.data.activeTargets[] | select(.labels.juju_application == "controller")'
}

test_prometheus() {
	if [ "$(skip 'test_prometheus')" ]; then
		echo "==> TEST SKIPPED: Prometheus integration"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"k8s")
			run "run_prometheus"
			run "run_prometheus_multiple_units"
			;;
		*)
			echo "==> TEST SKIPPED: run_prometheus test runs on k8s only"
			echo "==> TEST SKIPPED: run_prometheus_multiple_units test runs on k8s only"
			;;
		esac

		run "run_prometheus_cross_controller"
		# TODO: test HA
	)
}
