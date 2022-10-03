# Ensure that COS Lite deploys successfully, the
# relations are setup as expected and we can access the dashboards.
run_deploy_coslite() {
	echo

	local model_name file overlay_path admin_passwd alertmanager_ip grafana_ip prometheus_ip
	model_name="deploy-coslite"
	file="${TEST_DIR}/${model_name}.log"
	ensure "${model_name}" "${file}"

	overlay_path="./tests/suites/coslite/overlay"
	juju deploy cos-lite --trust --channel=beta --overlay "${overlay_path}/offers-overlay.yaml" --overlay "${overlay_path}/storage-small-overlay.yaml"

	echo "Wait for all unit agents to be in active condition"
	wait_for 0 '[.applications[] | select((.units[] | .["juju-status"].current != "idle") and (.units[] | .["workload-status"].current != "error"))] | length' 1800

	echo "Check that all offer endpoints specified in the overlays exist"
	wait_for 5 '[.offers[] | .endpoints] | length'

	admin_passwd=$(juju run-action --wait grafana/0 get-admin-password --format json | jq '.["unit-grafana-0"]["results"]["admin-password"]')
	if [ -z "$admin_passwd" ]; then
		echo "expected to get admin password for grafana/0"
		exit 1
	fi

	# Assert the web dashboards are reachable
	alertmanager_ip=$(juju status --format json | jq '.applications["alertmanager"]["units"]["alertmanager/0"].address' | tr -d '"')
	echo "Check if alertmanager is ready to serve traffic"
	curl -sS http://"$alertmanager_ip":9093/ready -o /dev/null
	grafana_ip=$(juju status --format json | jq '.applications["grafana"]["units"]["grafana/0"].address' | tr -d '"')
	echo "Check if grafana is ready to serve traffic"
	curl -sS http://"$grafana_ip":3000/api/health -o /dev/null
	prometheus_ip=$(juju status --format json | jq '.applications["prometheus"]["units"]["prometheus/0"].address' | tr -d '"')
	echo "check if prometheus is ready to serve traffic"
	curl -sS http://"$prometheus_ip":9090/ready -o /dev/null
	echo "cos lite tests passed"

	# without --force grafana get stuck in a hook(removal) error state.
	juju remove-application grafana --force

	destroy_model "${model_name}"
}

test_deploy_coslite() {
	if [ "$(skip 'test_deploy_coslite')" ]; then
		echo "==> TEST SKIPPED: Test Deploy coslite"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_coslite"
	)
}
