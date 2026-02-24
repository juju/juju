# Exercising the CMR offer permissions functionality which allows admins to
# manage user access to application offers.
# This test will exercise the following aspects:
#  - Ensure a user can create an offer of an application's endpoint
#  - Ensure multiple users can be granted consume access to an offer
#  - Ensure show-offer correctly displays user permissions in multiple formats:
#      - JSON format with user access levels
#      - YAML format with user access levels
#      - Plain text format
#  - Ensure list-offers correctly shows available offers
#  - Ensure removing a user updates the offer permissions correctly
#
# The above feature tests will be run on:
#  - A single controller environment
run_offer_permissions() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-offer-permissions.log"
	ensure "model-offer" "${file}"

	echo "Deploy consumed workload and create the offer"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink dummy-offer

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Check list-offer output"
	juju list-offers --format=json | jq -r 'has("dummy-offer")' | check true

	echo "Check show-offer plain output"
	check_contains "$(juju show-offer dummy-offer)" "admin/model-offer.dummy-offer"

	echo "Check show-offer json output"
	offer_json=$(juju show-offer dummy-offer --format=json)
	offer_id="${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer.dummy-offer"
	jq "has(\"$offer_id\")" <<<$offer_json | check true
	jq ".\"$offer_id\".users | has(\"admin\")" <<<$offer_json | check true
	jq ".\"$offer_id\".users | keys | length" <<<$offer_json | check 1
	jq -r ".\"$offer_id\".users.admin.\"display-name\"" <<<$offer_json | check "admin"

	echo "Add permissions for some users"
	for i in $(seq 1 5); do juju add-user user$i; done
	for i in $(seq 1 5); do juju grant user$i consume admin/model-offer.dummy-offer; done

	echo "Check show-offer json output with multiple users"
	offer_json=$(juju show-offer dummy-offer --format=json)
	jq "has(\"$offer_id\")" <<<$offer_json | check true
	jq ".\"$offer_id\".users | has(\"admin\")" <<<$offer_json | check true
	jq ".\"$offer_id\".users | keys | length" <<<$offer_json | check 6
	jq -r ".\"$offer_id\".users.admin.\"display-name\"" <<<$offer_json | check "admin"
	jq -r ".\"$offer_id\".users.admin.access" <<<$offer_json | check "admin"
	for i in $(seq 1 5); do jq -r ".\"$offer_id\".users.user$i.access" <<<$offer_json | check "consume"; done

	echo "Check show-offer yaml output"
	offer_yaml=$(juju show-offer dummy-offer --format=yaml)
	yq "has(\"$offer_id\")" <<<$offer_yaml | check true
	yq ".\"$offer_id\".users | has(\"admin\")" <<<$offer_yaml | check true
	yq ".\"$offer_id\".users | keys | length" <<<$offer_yaml | check 6
	yq -r ".\"$offer_id\".users.admin.\"display-name\"" <<<$offer_yaml | check "admin"
	yq -r ".\"$offer_id\".users.admin.access" <<<$offer_yaml | check "admin"
	for i in $(seq 1 5); do yq -r ".\"$offer_id\".users.user$i.access" <<<$offer_yaml | check "consume"; done

	echo "Remove user3 and check show-offer yaml output"
	juju remove-user user3 -y
	offer_yaml=$(juju show-offer dummy-offer --format=yaml)
	yq ".\"$offer_id\".users | keys | length" <<<$offer_yaml | check 5
	yq ".\"$offer_id\".users | has(\"user3\")" <<<$offer_yaml | check false
	yq -r ".\"$offer_id\".users.admin.access" <<<$offer_yaml | check "admin"
	for i in 1 2 4 5; do yq -r ".\"$offer_id\".users.user$i.access" <<<$offer_yaml | check "consume"; done

	echo "Clean up"
	destroy_model "model-offer"
}

test_offer_permissions() {
	if [ "$(skip 'test_offer_permissions')" ]; then
		echo "==> TEST SKIPPED: offer permissions"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_offer_permissions"
	)
}
