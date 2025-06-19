run_smoke_test() {
	# Echo out to ensure nice output to the test suite.
	echo

	file="${TEST_DIR}/test-smoke.log"
	ensure "smoke" "${file}"

	juju add-machine
	juju exec --machine 0 -- 'sudo mkdir -p /var/lib/juju/wrench; sudo sh -c "echo "short-interval" > /var/lib/juju/wrench/metricscollector"'
	juju deploy ./testcharms/charms/metered --to 0

	wait_for "metered" "$(idle_condition "metered")"

	attempt=0
	while true; do
		OUT=$(juju metrics metered/0 --format json | jq -r '.[]')
		if [[ -n ${OUT} ]]; then
			break
		fi
		attempt=$((attempt + 1))
		if [ $attempt -eq 10 ]; then
			# shellcheck disable=SC2046
			echo $(red "timeout: waiting for unit metrics")
			exit 1
		fi
		sleep 3
	done

	UNIT=$(echo $OUT | jq .unit)
	check_contains "$UNIT" "metered/0"
	METRIC=$(echo $OUT | jq .metric)
	check_contains "$METRIC" "juju-units"
	VALUE=$(echo $OUT | jq .value)
	check_contains "$VALUE" "1"

	destroy_model "smoke"
}
