test_constraints_common() {
  if [ "$(skip 'test_constraints_common')" ]; then
  		echo "==> TEST SKIPPED: constraints common tests"
  		return
  fi

  (
  		set_verbosity

  		cd .. || exit

      case "${BOOTSTRAP_PROVIDER:-}" in
      "lxd" | "lxd-remote" | "localhost")
        run "run_constraints_lxd"
        ;;
      "aws" | "ec2")
        run "run_constraints_aws"
        ;;
      *)
        echo "==> TEST SKIPPED: constraints - tests for LXD and AWS only"
        ;;
      esac
  )
}
