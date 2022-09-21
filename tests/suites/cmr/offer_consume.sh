# Exercising the CMR Juju functionality which allows applications to
# communicate between different models.
# This test will exercise the following aspects:
#  - Ensure a user is able to create an offer of an applications' endpoint
#    including:
#      - A user is able to consume and relate to the offer
#      - Workload data successfully provided
#      - The offer appears in the list-offers output
#      - The user is able to name the offer
#      - The user is able to remove the offer
#  - Ensure an admin can grant a user access to an offer
#      - The consuming user finds the offer via 'find-offer'
#
# The above feature tests will be run on:
#  - A single controller environment
run_offer_consume() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-offer-consume.log"
	ensure "model-offer" "${file}"

	juju deploy juju-qa-dummy-source
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	juju add-model "model-consume"
	juju switch "model-consume"
	juju deploy juju-qa-dummy-sink

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju --show-log consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer.dummy-source"
	juju --show-log relate dummy-sink dummy-source
	# wait for relation joined before migrate.
	wait_for "dummy-source" '.applications["dummy-sink"] | .relations.source[0]'
	sleep 30

	# Change the dummy-source config for "token" and check that the change
	# is represented in the consuming model's dummy-sink unit.
	juju switch "model-offer"
	juju config dummy-source token=yeah-boi
	juju switch "model-consume"
	wait_for "active" ".\"application-endpoints\"[\"dummy-source\"].\"application-status\".current"

	# The offer must be removed before model/controller destruction will work.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju switch "model-offer"
	juju remove-offer "admin/model-offer.dummy-source" --force -y

	# Clean up.
	destroy_model "model-offer"
	destroy_model "model-consume"
}

test_offer_consume() {
	if [ "$(skip 'test_offer_consume')" ]; then
		echo "==> TEST SKIPPED: offer consume"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_offer_consume"
	)
}
