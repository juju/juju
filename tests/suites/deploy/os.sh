test_deploy_os() {
	if [ "$(skip 'test_deploy_os')" ]; then
		echo "==> TEST SKIPPED: deploy to os"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		case "${BOOTSTRAP_PROVIDER:-}" in
		"ec2" | "aws")
			#
			# A handy place to find the current AMIs for centos
			# https://wiki.centos.org/Cloud/AWS
			#
			run "run_deploy_centos7"
			;;
		*)
			echo "==> TEST SKIPPED: deploy_centos - tests for AWS only"
			;;
		esac
	)
}

run_deploy_centos7() {
	echo

	echo "==> Checking for dependencies"
	check_juju_dependencies metadata

	name="test-deploy-centos7"
	file="${TEST_DIR}/${name}.log"

	ensure "${name}" "${file}"

	#
	# Images have been setup and and subscribed for juju-qa aws
	# in us-west-2.  Take care editing the details.
	#
	juju add-model test-deploy-centos-west2 aws/us-west-2

	juju metadata add-image --series centos7 ami-0bc06212a56393ee1

	#
	# There is a specific list of instance types which can be used with
	# this image.  Sometimes juju chooses the wrong one e.g. t3a.medium.
	# Ensure we use one that is allowed.
	#
	juju deploy ./tests/suites/deploy/charms/centos-dummy-sink --series centos7 --constraints instance-type=t3.medium

	series=$(juju status --format=json | jq '.applications."dummy-sink".series')
	echo "$series" | check "centos7"

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	destroy_model "${name}"
	destroy_model "test-deploy-centos-west2"
}
