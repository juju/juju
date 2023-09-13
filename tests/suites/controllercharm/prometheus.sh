run_prometheus() {
	echo

	MODEL_NAME="test-prometheus"
	file="${TEST_DIR}/${MODEL_NAME}.log"
	bootstrap "${MODEL_NAME}" "${file}"

	juju offer controller.controller:metrics-endpoint

	juju deploy prometheus-k8s --trust
	juju relate prometheus-k8s controller.controller
	wait_for "prometheus-k8s" "$(active_idle_condition "prometheus-k8s" 0 0)"
	retry 'check_prometheus_targets prometheus-k8s 0' 10

	juju remove-relation prometheus-k8s controller
	# Check Juju controller is removed from Prometheus targets
	retry 'check_prometheus_no_target prometheus-k8s 0' 10
	# Check no errors in controller charm or Prometheus
	juju status -m controller --format json | jq -r "$(active_condition "controller")" | check "controller"
	juju status --format json | jq -r "$(active_condition "prometheus-k8s")" | check "prometheus-k8s"

	juju remove-application prometheus-k8s --destroy-storage \
		--force --no-wait # TODO: remove these flags once storage bug is fixed
	destroy_model "${MODEL_NAME}"
	destroy_controller "${MODEL_NAME}"
}

# Check the controller charm can handle multiple Prometheus relations.
run_prometheus_multi() {
	echo

	MODEL_NAME="test-prometheus-multi"
	file="${TEST_DIR}/${MODEL_NAME}.log"
	bootstrap "${MODEL_NAME}" "${file}"

	juju offer controller.controller:metrics-endpoint

	juju deploy prometheus-k8s p1 --trust
	juju relate p1 controller.controller
	wait_for "p1" "$(active_idle_condition "p1" 0 0)"
	retry 'check_prometheus_targets p1 0' 10

	juju deploy prometheus-k8s p2 --trust
	juju relate p2 controller.controller
	wait_for "p2" "$(active_idle_condition "p2" 0 0)"
	retry 'check_prometheus_targets p2 0' 10

	juju add-unit p1
	wait_for "p1" "$(active_idle_condition "p1" 0 1)"
	retry 'check_prometheus_targets p1 1' 10

	juju remove-unit p1
	# Check all applications are still healthy
	juju status -m controller --format json | jq -r "$(active_condition "controller")" | check "controller"
	juju status --format json | jq -r "$(active_condition "p1")" | check "p1"

	juju remove-relation p2 controller
	# Check Juju controller is removed from Prometheus targets
	retry 'check_prometheus_no_target p2 0' 10
	# Check no errors in controller charm or Prometheus
	juju status -m controller --format json | jq -r "$(active_condition "controller")" | check "controller"
	juju status --format json | jq -r "$(active_condition "p2")" | check "p2"

	juju remove-relation p1 controller
	# Check Juju controller is removed from Prometheus targets
	retry 'check_prometheus_no_target p1 0' 10
	# Check no errors in controller charm or Prometheus
	juju status -m controller --format json | jq -r "$(active_condition "controller")" | check "controller"
	juju status --format json | jq -r "$(active_condition "p1")" | check "p1"

	juju remove-application p1 --destroy-storage \
		--force --no-wait # TODO: remove these flags once storage bug is fixed
	juju remove-application p2 --destroy-storage \
		--force --no-wait # TODO: remove these flags once storage bug is fixed
	destroy_model "${MODEL_NAME}"
	destroy_controller "${MODEL_NAME}"
}

run_prometheus_cross_controller() {
	echo

	CONTROLLER_MODEL_NAME="test-controller"
	file="${TEST_DIR}/${CONTROLLER_MODEL_NAME}.log"
	bootstrap "${CONTROLLER_MODEL_NAME}" "${file}"

	PROMETHEUS_MODEL_NAME="test-prometheus"
	file="${TEST_DIR}/${PROMETHEUS_MODEL_NAME}.log"
	BOOTSTRAP_PROVIDER='k8s' BOOTSTRAP_CLOUD='microk8s' bootstrap "${PROMETHEUS_MODEL_NAME}" "${file}"

	juju offer controller.controller:metrics-endpoint

	juju deploy prometheus-k8s --trust
	juju relate prometheus-k8s controller.controller
	wait_for "prometheus-k8s" "$(active_idle_condition "prometheus-k8s" 0 0)"
	retry 'check_prometheus_targets prometheus-k8s 0' 10

	juju remove-relation prometheus-k8s controller
	# Check Juju controller is removed from Prometheus targets
	retry 'check_prometheus_no_target prometheus-k8s 0' 10
	# Check no errors in controller charm or Prometheus
	juju status -m controller --format json | jq -r "$(active_condition "controller")" | check "controller"
	juju status --format json | jq -r "$(active_condition "prometheus-k8s")" | check "prometheus-k8s"

	juju remove-application prometheus-k8s --destroy-storage \
		--force --no-wait # TODO: remove these flags once storage bug is fixed
	destroy_model "${PROMETHEUS_MODEL_NAME}"
	destroy_controller "${PROMETHEUS_MODEL_NAME}"
	destroy_model "${CONTROLLER_MODEL_NAME}"
	destroy_controller "${CONTROLLER_MODEL_NAME}"
}

# Check the Juju controller in the list of Prometheus targets.
#   usage: check_prometheus_targets <app-name> <unit-number>
check_prometheus_targets() {
	set -uo pipefail
	local app_name=$1
	local unit_number=$2

	TARGET=$(get_juju_target "$app_name" "$unit_number")
	if [[ -z $TARGET ]]; then
		echo "Juju controller not found in Prometheus targets"
		return 1
	fi

	TARGET_STATUS=$(echo $TARGET | jq -r '.health')
	if [[ $TARGET_STATUS != "up" ]]; then
		echo "Controller metrics endpoint status: $TARGET_STATUS: $(echo $TARGET | jq -r '.lastError')"
		return 1
	fi

	echo "Controller metrics endpoint is up"
}

# Check the Juju controller is not present in the list of Prometheus targets.
#   usage: check_prometheus_targets <app-name> <unit-number>
check_prometheus_no_target() {
	set -uo pipefail
	local app_name=$1
	local unit_number=$2

	TARGET=$(get_juju_target "$app_name" "$unit_number")
	if [[ -n $TARGET ]]; then
		echo "Whoops: Juju controller still found in Prometheus targets"
		return 1
	fi

	echo "Success: Juju controller removed from Prometheus targets"
}

# Extract the Juju controller from the list of Prometheus targets
#   usage: get_juju_target <app-name> <unit-number>
get_juju_target() {
	set -uo pipefail
	local app_name=$1
	local unit_number=$2

	PROM_IP=$(juju status --format json |
		jq -r ".applications.\"$app_name\".units.\"$app_name/$unit_number\".address")
	TARGET=$(curl -sSm 2 "http://${PROM_IP}:9090/api/v1/targets" |
		jq '.data.activeTargets[] | select(.labels.juju_application == "controller")')
	echo "$TARGET"
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
			run "run_prometheus_multi"
			;;
		*)
			echo "==> TEST SKIPPED: run_prometheus test runs on k8s only"
			echo "==> TEST SKIPPED: run_prometheus_multi test runs on k8s only"
			;;
		esac

		# TODO: test relating to multiple Prometheus instances at once
		# TODO: test cross-controller relation (lxd controller)
	)
}
