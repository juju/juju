# Ensure that Magma Orchestrator deploys successfully.
run_deploy_magma() {
	echo

	local name model_name file overlay_path cert_password nms_ip admin_username
	name="deploy-magma"
	model_name="${name}"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	echo "Deploy Magma project"
	overlay_path="./tests/suites/magma/overlay/overlay.yaml"
	juju deploy magma-orc8r --overlay "${overlay_path}" --trust --channel=beta

	echo "Check all Magma project components have ACTIVE status"
	# Magical number 34 means that all 34 apps from the magma bundle has the same status
	wait_for 34 '[.applications[] | select(."application-status".current == "active")] | length' 3600

	echo "Get cert file and request password for it"
	juju scp --container="magma-orc8r-certifier" orc8r-certifier/0:/var/opt/magma/certs/admin_operator.pfx "${TEST_DIR}/admin_operator.pfx"
	cert_password=$(juju run orc8r-certifier/leader get-pfx-package-password --wait --format=json | jq -r '."unit-orc8r-certifier-0".results.password')

	echo "Get IP address of NMS nginx proxy"
	nms_ip=$(juju run orc8r-orchestrator/leader get-load-balancer-services --wait --format=json | jq -r '."unit-orc8r-orchestrator-0".results."nginx-proxy"')

	echo "Try to get access to Magma web interface via cert"
	curl --insecure -s --cert-type P12 --cert "${TEST_DIR}"/admin_operator.pfx:"${cert_password}" https://"${nms_ip}":443 | jq -r ".errorCode" | check "USER_NOT_LOGGED_IN"

	echo "Get NMS admin username"
	admin_username=$(juju run nms-magmalte/leader get-master-admin-credentials --wait --format=json | jq -r '."unit-nms-magmalte-0".results."admin-username"')
	echo "${admin_username}" | check "admin@juju.com"

}

test_deploy_magma() {
	if [ "$(skip 'test_deploy_magma')" ]; then
		echo "==> TEST SKIPPED: Test Deploy Magma"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_deploy_magma"
	)
}
