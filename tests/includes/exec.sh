# juju_exec_output wraps `juju exec --format yaml` to reliably extract
# command stdout without mixing it with stderr (log noise, error messages
# from the coveruploader, juju-level ERROR output, etc.).
#
# Usage:
#   juju_exec_output [juju exec options] [--] <command>
#
# Examples:
#   value=$(juju_exec_output --unit hello/0 -- secret-add foo=bar)
#   juju_exec_output --all -- hostname
#   juju_exec_output -a myapp -- network-get myendpoint
#
# Behaviour:
#   - Success: prints the stdout of the executed command(s) to stdout.
#   - Failure (any task with non-zero return code): returns 1.
#   - Every task's stderr AND juju exec's own stderr (ERROR messages,
#     log noise) are always forwarded to stderr and never captured as data.
juju_exec_output() {
	local yaml_output juju_rc fail_count

	# Capture juju exec stdout (YAML) while forwarding its stderr to the
	# caller's stderr in real time. FD 3 bridges the subshell's stderr to
	# the outer stderr: the block sets 3>&2, then inside $() juju exec's
	# stderr goes to FD 3 (2>&3) and FD 3 is closed (3>&-) so it is not
	# captured.
	{
		yaml_output=$(juju exec --format yaml "$@" 2>&3 3>&-) && juju_rc=0 || juju_rc=$?
	} 3>&2

	# Hard failure: juju produced no parseable YAML. Surface exit code only.
	if [[ -z ${yaml_output} ]]; then
		return "${juju_rc}"
	fi

	# Forward stderr from ALL tasks to real stderr.
	printf '%s\n' "${yaml_output}" |
		yq -r '.[].results.stderr // "" | select(. != "")' >&2 2>/dev/null

	# Count tasks with a non-zero (or missing) return code. Quoted key
	# access avoids any bare-hyphen ambiguity, and a null result counts
	# as a failure.
	fail_count=$(printf '%s\n' "${yaml_output}" |
		yq '[.[] | select(.results["return-code"] != 0)] | length' 2>/dev/null) || true

	if ((${fail_count:-0} > 0)); then
		return 1
	fi

	# Emit stdout from all results (target order preserved).
	printf '%s\n' "${yaml_output}" | yq -r '.[].results.stdout // ""'
}
