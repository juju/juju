# Ensure that relating two applications by name only prefers an endpoint
# declared by the charms over the implicit juju-info endpoint.
run_relation_implicit_endpoint() {
	echo

	model_name="test-relation-implicit-endpoint"
	file="${TEST_DIR}/${model_name}.log"

	ensure "${model_name}" "${file}"

	# The principal provides the link endpoint and, like every charm, the
	# implicit juju-info endpoint. The subordinate requires both, so two
	# endpoint pairs are compatible.
	juju deploy "$(pack_charm ./testcharms/charms/principal-link)" principal
	juju deploy "$(pack_charm ./testcharms/charms/subordinate-link)" subordinate

	echo "Integrate by application name: the declared endpoint is used"
	juju integrate principal subordinate
	wait_for "subordinate" '.applications["principal"] | .relations.link[0]["related-application"]'
	check "null" <<<"$(juju status --format=json | yq '.applications["principal"].relations["juju-info"]')"

	echo "Remove by application name"
	juju remove-relation principal subordinate
	wait_for null '.applications["principal"] | .relations'

	echo "Integrate naming the implicit endpoint: it can still be used"
	juju integrate principal:juju-info subordinate
	wait_for "subordinate" '.applications["principal"] | .relations["juju-info"][0]["related-application"]'
	check "null" <<<"$(juju status --format=json | yq '.applications["principal"].relations.link')"

	echo "Remove by application name: the existing relation is found"
	juju remove-relation principal subordinate
	wait_for null '.applications["principal"] | .relations'

	destroy_model "${model_name}"
}

test_relation_implicit_endpoint() {
	if [ "$(skip 'test_relation_implicit_endpoint')" ]; then
		echo "==> TEST SKIPPED: relation implicit endpoint tests"
		return
	fi

	(
		set_verbosity

		cd .. || exit

		run "run_relation_implicit_endpoint"
	)
}
