# Ensure that COS Lite deploys successfully, the
# relations are setup as expected and we can access the dashboards.
run_deploy_coslite() {
	echo

	local model_name file admin_passwd alertmanager_ip grafana_ip prometheus_ip
	model_name="deploy-coslite"
	file="${TEST_DIR}/${model_name}.log"
	ensure "${model_name}" "${file}"

	juju deploy ubuntu-lite # deploy ubuntu-lite for a container that can easily access cos-lite components on all k8s
	juju deploy cos-lite --trust --channel=stable
	juju config traefik external_hostname=test-coslite.com
	echo "Wait for all unit agents to be in idle condition"
	wait_for 0 "$(not_idle_list) | length" 1800

	# run-action will change in 3.0
	admin_passwd=$(juju run grafana/0 get-admin-password --wait=2m --format json | jq '.["unit-grafana-0"]["results"]["admin-password"]')
	if [ -z "$admin_passwd" ]; then
		echo "expected to get admin password for grafana/0"
		exit 1
	fi

	echo "check if alertmanager is ready"
	alertmanager_ip=$(juju status --format=json | jq -r '.applications.alertmanager.units."alertmanager/0".address')
	check_ready "http://$alertmanager_ip:9093/-/ready" 200

	echo "check if grafana is ready"
	grafana_ip=$(juju status --format=json | jq -r '.applications.grafana.units."grafana/0".address')
	check_ready "http://$grafana_ip:3000/api/health" 200

	echo "check if prometheus is ready"
	prometheus_ip=$(juju status --format=json | jq -r '.applications.prometheus.units."prometheus/0".address')
	check_ready "http://$prometheus_ip:9090/-/ready" 200

	echo "cos lite tests passed"
}

# Check that curl request return needed code
check_ready() {
	local url code
	url=${1}
	code=${2}
	attempt=1
	while true; do
		status_code=$(juju ssh ubuntu-lite/0 curl --write-out "%{http_code}" -L --silent --output /dev/null "${url}")
		if [[ $status_code -eq $code ]]; then
			echo "Ready to serve traffic"
			break
		fi
		if [[ ${attempt} -ge 5 ]]; then
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
