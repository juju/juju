# A scenario to test external controller record updates:
#  1. A consumer model consumes two offers hosted by an offering controller.
#     The consumer's record of the offering controller has both
#     offered model UUIDs associated with it.
#  2. The consumer removes the saas for the first offer. The external
#     controller record on the consumer is retained (the second offer is
#     still consumed) and still lists the migrated model's UUID.
#  3. The offering model is migrated to a third controller.
#  4. The consumer consumes the offer again, this time from the new
#     controller, and relates to it.
run_offer_consume_migrate() {
	# Echo out to ensure nice output to the test suite.
	echo

	# The following ensures that a bootstrap juju exists.
	file="${TEST_DIR}/test-offer-consume-migrate.log"
	ensure "model-offer-migrate" "${file}"

	echo "Bootstrap consumer and migration target controllers"
	bootstrap_alt_controller "cmr-consume"
	bootstrap_alt_controller "cmr-migrate"

	echo "Deploy two offered applications on the offering controller"
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	add_model "model-offer-1"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink offer-one
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	add_model "model-offer-2"
	juju deploy juju-qa-dummy-source --base ubuntu@22.04
	juju offer dummy-source:sink offer-two
	wait_for "dummy-source" "$(idle_condition "dummy-source")"

	echo "Deploy workload in the consumer model and consume both offers"
	juju switch "cmr-consume"
	add_model "model-consume"
	juju deploy juju-qa-dummy-sink --base ubuntu@22.04

	wait_for "dummy-sink" "$(idle_condition "dummy-sink")"

	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-1.offer-one"
	juju relate dummy-sink offer-one
	# Wait for relation joined before migrating.
	wait_for "offer-one" '.applications["dummy-sink"] | .relations.source[0]'

	# Keep the offering controller's external controller record on the
	# consumer alive. The external controller record also holds the
	# model-offer-1 model UUID until it is explicitly moved.
	juju consume "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-2.offer-two"

	echo "Remove the offer-one saas, then migrate its offering model"
	juju remove-relation dummy-sink offer-one
	juju remove-saas offer-one
	# The offer-two saas is retained, so the offering controller record persists.
	wait_for null '.applications["dummy-sink"] | .relations'
	wait_for null '.["application-endpoints"]["offer-one"]'

	# The cross-model relation must be fully torn down on the offering side as
	# well, otherwise migration prechecks fail with a unit "hasn't joined
	# relation" error (see the workaround in model/migration.sh).
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:model-offer-1"
	wait_for null '.relations'
	wait_for null '.offers["offer-one"]."total-connected-count"'

	# The offering model is migrated to a new controller. The consumer has no
	# saas for offer-one any more, so nothing updates its external controller info
	# via a migration redirect.
	echo "Migrate the offering model to the new controller"
	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	if ! migrate_out=$(juju migrate "model-offer-1" "cmr-migrate" 2>&1); then
		echo "$(red 'migrate failed')"
		echo "${migrate_out}" | sed 's/^/    | /'
		exit 1
	fi

	# Wait for the model to appear on the target controller.
	echo "Wait for the model to appear on the target controller"
	attempt=0
	until [ "$(juju models --controller cmr-migrate --format=json 2>/dev/null | yq -r '.models[] | .["short-name"]' | grep -E '^model-offer-1$')" ]; do
		echo "[+] (attempt ${attempt}) waiting for migration of model-offer-1 to cmr-migrate"
		juju models --controller cmr-migrate 2>/dev/null | sed 's/^/    | /'
		sleep 5
		attempt=$((attempt + 1))
		if [ "${attempt}" -ge 120 ]; then
			echo "$(red 'timed out waiting for model-offer-1 to migrate to cmr-migrate')"
			exit 1
		fi
	done

	echo "Re-consume the offer from the new controller"
	juju switch "cmr-consume:model-consume"
	juju consume "cmr-migrate:admin/model-offer-1.offer-one"
	juju relate dummy-sink offer-one
	wait_for "offer-one" '.applications["dummy-sink"] | .relations.source[0]'

	echo "Verify data still flows through the re-created saas"
	# Change the dummy-source config for "token" and check that the change
	# is represented in the consuming model's dummy-sink unit.
	juju switch "cmr-migrate:model-offer-1"
	juju config dummy-source token=yeah-boi
	juju switch "cmr-consume:model-consume"
	wait_for "yeah-boi" "$(workload_status "dummy-sink" 0).message"

	echo "Clean up"
	# Offers must be removed before controller destruction will work.
	# See discussion under https://bugs.launchpad.net/juju/+bug/1830292.
	juju switch "cmr-consume:model-consume"
	juju remove-relation dummy-sink offer-one
	juju remove-saas offer-one
	juju remove-saas offer-two

	juju switch "cmr-migrate:model-offer-1"
	wait_for null '.offers["offer-one"]."total-connected-count"'
	juju remove-offer "cmr-migrate:admin/model-offer-1.offer-one" --force -y

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}:model-offer-2"
	wait_for null '.offers["offer-two"]."total-connected-count"'
	juju remove-offer "${BOOTSTRAPPED_JUJU_CTRL_NAME}:admin/model-offer-2.offer-two" --force -y

	destroy_controller "cmr-consume"
	destroy_controller "cmr-migrate"

	juju switch "${BOOTSTRAPPED_JUJU_CTRL_NAME}"
	destroy_model "model-offer-1"
	destroy_model "model-offer-2"
}

test_offer_consume_migrate() {
	if [ "$(skip 'test_offer_consume_migrate')" ]; then
		echo "==> TEST SKIPPED: offer consume migrate"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_offer_consume_migrate"
	)
}
