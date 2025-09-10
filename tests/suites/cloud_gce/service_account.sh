run_serviceaccount_credential() {
	echo

	file="${TEST_DIR}/test-serviceaccount-credential.log"

	export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} --bootstrap-constraints=instance-role=auto"
	bootstrap "test-serviceaccount-gce" "${file}"

	projectServiceAccount=$(gcloud compute project-info describe --format json | jq -r .defaultServiceAccount)
	credServiceAccount=$(juju show-credential --controller "$BOOTSTRAPPED_JUJU_CTRL_NAME" | yq '.controller-credentials .google .default .content .service-account')
	check_contains "$credServiceAccount" "$projectServiceAccount"

	juju switch "test-serviceaccount-gce"
	juju deploy ubuntu
	wait_for "ubuntu" "$(idle_condition "ubuntu")"

	juju switch controller
	juju enable-ha
	wait_for_controller_machines 3
	wait_for_ha 3

	for m in "0" "1" "2"; do
		instId=$(juju show-machine $m | yq '.machines .'"$m"' .instance-id')
		az=$(juju show-machine $m | yq '.machines .'"$m"' .hardware' | awk '{ delete vars; for(i = 1; i <= NF; ++i) { n = index($i, "="); if(n) { vars[substr($i, 1, n - 1)] = substr($i, n + 1) } } az = vars["availability-zone"] } { print az }')
		instServiceAccount=$(gcloud compute instances describe --zone "${az}" "${instId}" --format json | jq -r '.serviceAccounts[0].email')
		check_contains "$instServiceAccount" "$projectServiceAccount"
	done

	destroy_controller "test-serviceaccount-gce"
}

test_serviceaccount_credential() {
	if [ "$(skip 'test_serviceaccount_credential')" ]; then
		echo "==> TEST SKIPPED: service account credential"
		return
	fi

	setup_gcloudcli_credential

	set_verbosity

	echo "==> Checking for dependencies"
	check_dependencies juju gcloud

	cd .. || exit

	run "run_serviceaccount_credential" "$@"
}
