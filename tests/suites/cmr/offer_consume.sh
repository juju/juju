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
	juju list-offers --format=json | yq -r 'has("dummy-offer")' | check true

	echo "Deploy workload in consume model"
	juju add-model "model-consume"
	juju switch "model-consume"
	juju deploy juju-qa-dummy-sink --series jammy

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	echo "Check find-offer output"
	juju find-offers --format=json | yq -r "has(\"${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer.dummy-offer\")" | check true

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

	offer_controller="$(juju controllers --format=json | yq -r '."current-controller"')"

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

# run_offer_find_non_admin tests that a non-admin user who has been granted read
# access to an offer can successfully run 'juju find-offers' and see the offer.
run_offer_find_non_admin() {
	echo

	file="${TEST_DIR}/test-offer-find-non-admin.log"
	ensure "model-offer-find" "${file}"

	echo "Deploy application and create the offer"
	juju deploy juju-qa-dummy-source --series jammy
	juju offer dummy-source:sink dummy-offer

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Create non-admin user and register them"
	juju remove-user -y offeruser 2>/dev/null || true
	OUT=$(juju add-user offeruser)
	REG_CMD=$(echo "$OUT" | grep register | xargs)

	rm -rf /tmp/offeruser || true
	mkdir -p /tmp/offeruser
	# Inputs: password, confirm password, controller name, login password
	printf 'secret\nsecret\ntest-offer-find\nsecret\n' | JUJU_DATA=/tmp/offeruser ${REG_CMD}

	echo "Grant non-admin user read access to the offer"
	juju grant offeruser read "admin/model-offer-find.dummy-offer"

	echo "Check find-offers output as non-admin user"
	# JUJU_MODEL provides a controller:model qualifier so that find-offers can
	# resolve the controller name without requiring a current model to be set
	# (the non-admin user has no models of their own).

	JUJU_MODEL="test-offer-find:admin/model-offer-find" JUJU_DATA=/tmp/offeruser \
		juju find-offers --format=json |
		jq -r 'has("test-offer-find:admin/model-offer-find.dummy-offer")' |
		check true

	echo "Clean up"
	rm -rf /tmp/offeruser
	destroy_model "model-offer-find"
}

# run_offer_find_external_user verifies that an @external identity-provider user
# can run 'juju find-offers' and see available offers.  It starts a minimal
# bakery discharger (scripts/test-identity-provider) that auto-approves every
# login request as "testextuser", bootstraps a dedicated controller configured
# to use that discharger as its identity-url, and then logs in with a fresh
# JUJU_DATA (simulating the external-user experience) to confirm find-offers
# returns the expected offer.
run_offer_find_external_user() {
	echo

	# Start the discharger in the background; wait for it to write its two
	# output lines (URL then public key).
	IDP_OUTPUT="${TEST_DIR}/idp-output.txt"
	go run github.com/juju/juju/tests/tools/test-identity-provider --username testextuser >"${IDP_OUTPUT}" 2>&1 &
	track_daemon_pid $!

	IDP_URL=""
	IDP_PUBKEY=""
	for i in $(seq 1 20); do
		if [[ $(wc -l <"${IDP_OUTPUT}" 2>/dev/null) -ge 2 ]]; then
			IDP_URL=$(sed -n '1p' "${IDP_OUTPUT}")
			IDP_PUBKEY=$(sed -n '2p' "${IDP_OUTPUT}")
			break
		fi
		sleep $i
	done

	if [[ -z ${IDP_URL} || -z ${IDP_PUBKEY} ]]; then
		echo "ERROR: test identity provider failed to start"
		exit 1
	fi
	echo "Identity provider running at ${IDP_URL}"

	# Bootstrap a dedicated controller that uses the test identity provider.
	# BOOTSTRAP_ADDITIONAL_ARGS is extended here; pre_bootstrap will append
	# --agent-version etc., and post_bootstrap will unset it afterwards.
	export BOOTSTRAP_ADDITIONAL_ARGS="${BOOTSTRAP_ADDITIONAL_ARGS:-} \
		--config identity-url=${IDP_URL} \
		--config identity-public-key=${IDP_PUBKEY} \
		--config allow-model-access=true"

	file="${TEST_DIR}/test-offer-find-external.log"
	juju_bootstrap "${BOOTSTRAP_CLOUD:-localhost}" "ctrl-extuser-idp" "model-offer-ext" "${file}"

	# Grant the external user controller-level access so they can query offers.
	juju grant testextuser@external superuser

	echo "Deploy application and create the offer as admin"
	juju deploy juju-qa-dummy-source --series jammy
	juju offer dummy-source:sink dummy-offer

	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	# Retrieve one of the API endpoints for the external-user login step.
	CTRL_ENDPOINT=$(juju show-controller ctrl-extuser-idp --format=json |
		yq -r '."ctrl-extuser-idp".details."api-endpoints"[0]')

	echo "Login as testextuser@external (auto-approved by test identity provider)"
	TEST_EXTUSER_DIR="$(mktemp -d)"

	# --no-prompt suppresses all interactive input including the CA cert trust
	# prompt; --trust auto-approves the self-signed controller certificate.
	# The bakery discharger auto-approves the login, so no browser is needed.
	JUJU_DATA="${TEST_EXTUSER_DIR}" juju login "${CTRL_ENDPOINT}" \
		-c ctrl-extuser-idp --no-prompt --trust

	echo "Check find-offers output as external user"
	JUJU_MODEL="ctrl-extuser-idp:admin/model-offer-ext" JUJU_DATA="${TEST_EXTUSER_DIR}" \
		juju find-offers --format=json |
		yq -r 'has("ctrl-extuser-idp:admin/model-offer-ext.dummy-offer")' |
		check true

	echo "Clean up"
	rm -rf "${TEST_EXTUSER_DIR}"
	destroy_controller "ctrl-extuser-idp"
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

test_offer_find_external_user() {
	if [ "$(skip 'test_offer_find_external_user')" ]; then
		echo "==> TEST SKIPPED: offer find external user"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_offer_find_external_user"
	)
}

test_offer_find_non_admin() {
	if [ "$(skip 'test_offer_find_non_admin')" ]; then
		echo "==> TEST SKIPPED: offer find non-admin"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_offer_find_non_admin"
	)
}
