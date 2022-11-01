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
	wait_for 0 "$(idle_list)" 1800

	# run-action will change in 3.0
	admin_passwd=$(juju run-action --wait grafana/0 get-admin-password --format json | jq '.["unit-grafana-0"]["results"]["admin-password"]')
	if [ -z "$admin_passwd" ]; then
		echo "expected to get admin password for grafana/0"
		exit 1
	fi

	assert_dashboards
	echo "cos lite tests passed"
	# TODO(basebandit): enable destroy-model once model teardown has been fixed for k8s models.
	#destroy_model "${model_name}"
}

assert_dashboards() {
	# Assert the web dashboards are reachable
	traefik_ip=$(juju status --format json | jq '.applications["traefik"]["units"]["traefik/0"].address' | tr -d '"')
	get_proxied_unit_url "prometheus" 0
	prometheus_url_path=$(get_proxied_unit_url "prometheus" 0 | awk -F / '{print "/"$NF}')
	echo "check if prometheus is ready to serve traffic"
	# Replace the external, metallb URL with the one of traefik, to skip a lot of routing 'fun' (avoids tests failing intermittently)
	check_ready http://"$traefik_ip":80"$prometheus_url_path"/-/ready 200

	echo "Check if alertmanager is ready to serve traffic"
	alertmanager_ip=$(juju status --format json | jq '.applications["alertmanager"]["units"]["alertmanager/0"].address' | tr -d '"')
	# TODO(basebandit): Change this when AM is exposed over the ingress
	check_ready http://"$alertmanager_ip":9093 200

	echo "Check if grafana is ready to serve traffic"
	grafana_ip=$(juju status --format json | jq '.applications["grafana"]["units"]["grafana/0"].address' | tr -d '"')
	check_ready http://"$grafana_ip":3000 200
}

get_proxied_unit_url() {
	local unit unit_num
	unit=${1}
	unit_num=${2}
	proxied_endpoints=$(juju run-action --wait traefik/0 show-proxied-endpoints --format json | jq -r '.[] | .results["proxied-endpoints"]')
	echo "$proxied_endpoints" | jq ".[\"$unit/$unit_num\"][\"url\"]" | tr -d '"'
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
