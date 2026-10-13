# Helpers asserting that the machine agent's agent.conf carries
# connectivity state only: the tag, API addresses, credential, and trust
# anchor. Controller serving information belongs to the controller's
# runtime.conf, and Dqlite settings are delivered through runtime.conf.
#
# Entry point: the tail of run_charmhub_deploy in deploy.sh, immediately
# after its wait_for calls prove the bootstrapped controller answers
# juju status.

# machine0_agent_conf reads machine-0's agent.conf from the controller
# model. The output must never be logged or echoed by callers: it holds
# the machine credential. Only the key-probing helpers below consume it,
# and they report key names, never values.
machine0_agent_conf() {
	# shellcheck disable=SC2046
	# Not using juju_exec_output: sudo cat produces no stderr, so the
	# stdout/stderr mixing bug does not apply.
	juju exec -m controller --machine 0 'sudo cat /var/lib/juju/agents/machine-0/agent.conf'
}

# conf_key_line prints the top-level YAML line for the given key, if the
# key is present in the configuration body.
conf_key_line() {
	printf "%s" "${2}" | grep -E "^${1}:" || true
}

# assert_conf_key_present fails with the key name only. Never pass
# configuration values to the error path: the configuration body holds
# credentials.
assert_conf_key_present() {
	local line
	line=$(conf_key_line "${1}" "${2}")
	if [[ -z ${line} ]]; then
		echo "machine-0 agent.conf is missing expected key \"${1}\"" >&2
		return 1
	fi
	echo "Success: agent.conf has key \"${1}\"" >&2
}

# assert_conf_key_absent fails with the key name only.
assert_conf_key_absent() {
	local line
	line=$(conf_key_line "${1}" "${2}")
	if [[ -n ${line} ]]; then
		echo "machine-0 agent.conf must not contain key \"${1}\"" >&2
		return 1
	fi
	echo "Success: agent.conf has no key \"${1}\"" >&2
}

# assert_machine0_agent_conf_trimmed proves machine-0's agent.conf holds
# only machine-agent connectivity state: the required connectivity keys
# are present and no controller-serving, controller-password, or Dqlite
# field is serialized.
assert_machine0_agent_conf_trimmed() {
	local conf
	conf=$(machine0_agent_conf)

	# Sanity: the machine-0 machine agent configuration was read.
	local tag_line
	tag_line=$(conf_key_line "tag" "${conf}")
	if [[ ${tag_line} != "tag: machine-0" ]]; then
		echo "unexpected machine-0 agent.conf tag \"${tag_line}\"" >&2
		return 1
	fi

	# Required connectivity state: addresses, credential, trust anchor.
	local addresses_line
	addresses_line=$(conf_key_line "apiaddresses" "${conf}")
	if [[ -z ${addresses_line} || ${addresses_line} == "apiaddresses: []" ]]; then
		echo "machine-0 agent.conf has no API addresses" >&2
		return 1
	fi
	assert_conf_key_present "apipassword" "${conf}"
	assert_conf_key_present "cacert" "${conf}"

	# Controller serving information is owned by jujud runtime.conf.
	assert_conf_key_absent "controllercert" "${conf}"
	assert_conf_key_absent "controllerkey" "${conf}"
	assert_conf_key_absent "caprivatekey" "${conf}"
	assert_conf_key_absent "apiport" "${conf}"
	assert_conf_key_absent "systemidentity" "${conf}"

	# Controller-only password and legacy state-address fields are gone.
	assert_conf_key_absent "statepassword" "${conf}"
	assert_conf_key_absent "stateaddresses" "${conf}"

	# Dqlite settings are delivered via runtime.conf, not agent.conf.
	assert_conf_key_absent "querytracingenabled" "${conf}"
	assert_conf_key_absent "querytracingthreshold" "${conf}"
	assert_conf_key_absent "dqlitebusytimeout" "${conf}"
	assert_conf_key_absent "dqlite-port" "${conf}"
}
