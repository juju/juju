# run_constraints_model_bootstrap is asserting that model constraints applied at
# bootstrap time are both present and applied to the resultant model and also
# the controller application. We also test the controller machine for the
# presence of the constraint.
run_constraints_model_bootstrap() {
	(
		log_file="${TEST_DIR}/bootstrap.log"

		bootstrap_additional_args=(--constraints "mem=1024M")
		# The default public-cloud bootstrap sizing would override the model
		# constraints on machine 0 (bootstrap constraints take precedence),
		# so disable it for this bootstrap.
		PUBLIC_CLOUD_BOOTSTRAP_CONSTRAINTS="" \
			BOOTSTRAP_ADDITIONAL_ARGS="${bootstrap_additional_args[*]}" \
			BOOTSTRAP_REUSE=false \
			bootstrap "model-constraints" "$log_file"

		juju switch controller
		check_contains "$(juju model-constraints)" "mem=1024M"
		check_contains "$(juju constraints controller)" "mem=1024M"

		case "${BOOTSTRAP_CLOUD:-}" in
		"microk8s") ;;
		*)
			check_contains "$(juju show-machine 0)" "mem.*1024M"
			;;
		esac

		destroy_controller "model-constraints"
	)
}
