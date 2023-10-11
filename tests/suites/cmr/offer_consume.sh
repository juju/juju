# Exercising the CMR Juju functionality which allows applications to
# communicate between different models.
# This test will exercise the following aspects:
#  - Ensure a user is able to create an offer of an application's endpoint
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

	echo "Deploy consumed workload and create the offer"
	juju deploy juju-qa-dummy-source --series jammy
	juju offer dummy-source:sink dummy-offer

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Check list-offer output"
	juju list-offers --format=json | jq -r 'has("dummy-offer")' | check true

	echo "Deploy workload in consume model"
	juju add-model "model-consume"
	juju switch "model-consume"
	juju deploy juju-qa-dummy-sink --series jammy

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	echo "Check find-offer output"
	juju find-offers --format=json | jq -r "has(\"${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer.dummy-offer\")" | check true

	echo "Relate workload in consume model with offer"
	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer.dummy-offer"
	juju relate dummy-sink dummy-offer
	# wait for relation joined.
	wait_for "dummy-offer" '.applications["dummy-sink"] | .relations.source[0]'

	echo "Provide config for offered workload and change the status of consumed offer"
	# Change the dummy-source config for "token" and check that the change
	# is represented in the consuming model's dummy-sink unit.
	juju switch "model-offer"
	juju config dummy-source token=yeah-boi
	juju switch "model-consume"
	wait_for "active" '."application-endpoints"["dummy-offer"]."application-status".current'

	echo "Remove offer"
	juju remove-relation dummy-sink dummy-offer
	# wait for the relation to be removed.
	wait_for null '.applications["dummy-sink"] | .relations'
	juju remove-saas dummy-offer
	# wait for saas to be removed.
	wait_for null '.["application-endpoints"]'
	# The offer must be removed before model/controller destruction will work.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju switch "model-offer"
	wait_for null '.offers."dummy-offer"."total-connected-count"'
	juju remove-offer "admin/model-offer.dummy-offer" -y
	wait_for null '.offers'

	echo "Clean up"
	destroy_model "model-offer"
	destroy_model "model-consume"
}

# Previous test's features will be run on multiple controllers
# where each controller is in a different cloud.
run_offer_consume_cross_controller() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-offer-consume-cross-controller.log"
	ensure "model-offer" "${file}"

	offer_controller="$(juju controllers --format=json | jq -r '."current-controller"')"

	# Ensure we have another controller available.
	echo "Bootstrap consume offer controller"
	bootstrap_alt_controller "controller-consume"

	echo "Deploy consumed workload and create the offer"
	juju switch "${offer_controller}"
	juju deploy juju-qa-dummy-source --series jammy
	juju offer dummy-source:sink

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Deploy workload in consume controller"
	juju switch "controller-consume"
	juju add-model "model-consume"
	juju switch "model-consume"
	juju deploy juju-qa-dummy-sink --series jammy

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	echo "Relate workload in consume controller with offer"
	juju consume "${offer_controller}:admin/model-offer.dummy-source"
	juju relate dummy-sink dummy-source
	# wait for relation joined.
	wait_for "dummy-source" '.applications["dummy-sink"] | .relations.source[0]'

	echo "Provide config if offered workload and change the status of consumed offer"
	# Change the dummy-source config for "token" and check that the change
	# is represented in the consuming model's dummy-sink unit.
	juju switch "${offer_controller}:model-offer"
	juju config dummy-source token=yeah-boi
	juju switch "controller-consume:model-consume"
	wait_for "active" '."application-endpoints"["dummy-source"]."application-status".current'

	echo "Remove offer"
	juju remove-relation dummy-sink dummy-source
	# wait for the relation to be removed.
	wait_for null '.applications["dummy-sink"] | .relations'
	juju remove-saas dummy-source
	# wait for saas to be removed.
	wait_for null '.["application-endpoints"]'
	# The offer must be removed before model/controller destruction will work.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju switch "${offer_controller}:model-offer"
	wait_for null '.offers."dummy-offer"."total-connected-count"'
	juju remove-offer "${offer_controller}:admin/model-offer.dummy-source" -y
	wait_for null '.offers'

	echo "Clean up"
	destroy_controller "controller-consume"

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
		run "run_offer_consume_cross_controller"
	)
}
