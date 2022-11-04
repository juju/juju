# Ensure that COS Lite deploys successfully, the
# relations are setup as expected and we can access the dashboards.
run_deploy_coslite() {
	echo

	local model_name file overlay_path admin_passwd alertmanager_ip grafana_ip prometheus_ip
	model_name="deploy-coslite"
	file="${TEST_DIR}/${model_name}.log"
	ensure "${model_name}" "${file}"

	overlay_path="./tests/suites/coslite/overlay"
	juju deploy cos-lite --trust --channel=stable
	echo "Wait for all unit agents to be in idle condition"
	wait_for 0 "$(not_idle_count) | length" 1800

	# run-action will change in 3.0
	admin_passwd=$(juju run-action --wait grafana/0 get-admin-password --format json | jq '.["unit-grafana-0"]["results"]["admin-password"]')
	if [ -z "$admin_passwd" ]; then
		echo "expected to get admin password for grafana/0"
		exit 1
	fi

  echo "check if prometheus is ready"
	prometheus_url=$(get_proxied_unit_url "prometheus" 0)
	check_ready "http://$prometheus_url/-/ready" 200

	echo "check if catalogue is ready"
	catalogue_url=$(get_proxied_unit_url "catalogue")
	check_ready "http://$catalogue_url/" 200

  echo "check if loki is ready"
	loki_url=$(get_proxied_unit_url "loki" 0)
	check_ready "http://$loki_url/ready" 200

	alertmanager_url=$(get_proxied_unit_url "alertmanager")
	echo "Check if alertmanager is ready"
	check_ready "http://$alertmanager_url/-/ready" 200

	# TODO(basebandit): Change this when Grafana is exposed over the ingress
	grafana_ip=$(juju status --format json | jq -r '.applications["grafana"]["units"]["grafana/0"].address')
	echo "Check if grafana dashboard is reachable"
	check_ready "http://$grafana_ip:3000" 200

	echo "cos lite tests passed"
	# TODO(basebandit): enable destroy-model once model teardown has been fixed for k8s models.
	#destroy_model "${model_name}"
}

get_proxied_unit_url() {
	local unit unit_num path
	unit=${1}
	unit_num=${2}
	if [ -n "$unit_num" ]; then
		path=".[\"$unit/$unit_num\"][\"url\"]"
	else
		path=".[\"$unit\"][\"url\"]"
	fi
	proxied_endpoints=$(juju run-action --wait traefik/0 show-proxied-endpoints --format json | jq -r '.[] | .results["proxied-endpoints"]' | jq -r "$path")
	echo "$proxied_endpoints"
}

check_ready() {
	local url code
	url=${1}
	code=${2}
	attempt=1
	while true; do
		status_code=$(curl --write-out "%{http_code}" -L --silent --output /dev/null "${url}")
		if [[ $status_code -eq $code ]]; then
			echo "Ready to serve traffic"
			break
		fi
		if [[ ${attempt} -ge 3 ]]; then
			echo "Failed to connect to ${url} after ${attempt} attempts with status code ${status_code}"
			exit 1
		fi
		attempt=$((attempt + 1))
		sleep 5
	done
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
