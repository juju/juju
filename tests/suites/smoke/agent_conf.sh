# Helpers asserting that the machine agent's agent.conf carries
# connectivity state only: the tag, API addresses, credential, and trust
# anchor. Controller serving information belongs to the controller's
# runtime.conf, and Dqlite settings are delivered through runtime.conf.
#
# Entry point: the tail of run_charmhub_deploy in deploy.sh, immediately
# after its wait_for calls prove the bootstrapped controller answers
# juju status.

# machine0_agent_conf_keys prints the top-level key names in machine-0's
# agent.conf, one per line, with the values stripped. The file holds the
# machine credential, so the values never leave the controller machine;
# only key names are read here. Callers that need a value must use
# machine0_agent_conf_line with a non-secret key.
machine0_agent_conf_keys() {
	juju exec -m controller --machine 0 \
		"sudo grep -oE '^[A-Za-z][A-Za-z0-9_-]*:' /var/lib/juju/agents/machine-0/agent.conf"
}

# machine0_agent_conf_line prints the top-level YAML line for a
# non-secret key such as tag or apiaddresses. Never call it for a
# credential-bearing key.
machine0_agent_conf_line() {
	juju exec -m controller --machine 0 \
		"sudo grep -E '^${1}:' /var/lib/juju/agents/machine-0/agent.conf"
}

# conf_has_key reports whether the key-name list contains the given key.
conf_has_key() {
	printf "%s\n" "${2}" | grep -qx "${1}:"
}

# assert_conf_key_present fails with the key name only.
assert_conf_key_present() {
	if ! conf_has_key "${1}" "${2}"; then
		echo "machine-0 agent.conf is missing expected key \"${1}\"" >&2
		return 1
	fi
	echo "Success: agent.conf has key \"${1}\"" >&2
}

# assert_conf_key_absent fails with the key name only.
assert_conf_key_absent() {
	if conf_has_key "${1}" "${2}"; then
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
	local keys tag_line addresses_line
	keys=$(machine0_agent_conf_keys)

	# Sanity: the machine-0 machine agent configuration was read.
	tag_line=$(machine0_agent_conf_line "tag")
	if [[ ${tag_line} != "tag: machine-0" ]]; then
		echo "unexpected machine-0 agent.conf tag \"${tag_line}\"" >&2
		return 1
	fi

	# Required connectivity state: addresses, credential, trust anchor.
	addresses_line=$(machine0_agent_conf_line "apiaddresses")
	if [[ -z ${addresses_line} || ${addresses_line} == "apiaddresses: []" ]]; then
		echo "machine-0 agent.conf has no API addresses" >&2
		return 1
	fi
	assert_conf_key_present "apipassword" "${keys}"
	assert_conf_key_present "cacert" "${keys}"

	# Controller serving information is owned by jujud runtime.conf.
	assert_conf_key_absent "controllercert" "${keys}"
	assert_conf_key_absent "controllerkey" "${keys}"
	assert_conf_key_absent "caprivatekey" "${keys}"
	assert_conf_key_absent "apiport" "${keys}"
	assert_conf_key_absent "systemidentity" "${keys}"

	# Controller-only password and legacy state-address fields are gone.
	assert_conf_key_absent "statepassword" "${keys}"
	assert_conf_key_absent "stateaddresses" "${keys}"

	# Dqlite settings are delivered via runtime.conf, not agent.conf.
	assert_conf_key_absent "querytracingenabled" "${keys}"
	assert_conf_key_absent "querytracingthreshold" "${keys}"
	assert_conf_key_absent "dqlitebusytimeout" "${keys}"
	assert_conf_key_absent "dqlite-port" "${keys}"
}
