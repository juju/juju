// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"path/filepath"
	"text/template"

	"github.com/juju/errors"

	"github.com/juju/juju/cloudconfig/instancecfg"
)

// scriptArgs holds the possible argments passed to a bridge script.
type scriptArgs struct {
	// Config is the full path to the network config file, usually
	// /etc/network/interfaces.
	Config string

	// Bridge is the name of the bridge device to use, usually
	// instancecfg.DefaultBridgeName
	Bridge string

	// Commands contains full paths to the commands used by the scripts.
	Commands map[string]string

	// Scripts contains rendered script snippets included in the main script.
	Scripts map[string]string
}

const getGatewayAndPrimaryNICScript = `
# Discover the needed IPv4/IPv6 configuration for {{.Bridge}} (if any).
# Arguments:
#   $1: the first argument to {{.Commands.IP}} (e.g. "-6" or "-4")
# Outputs the discovered default gateway and primary NIC, separated
# with a space, if they could be discovered. The output is undefined
# otherwise.
get_gateway_and_primary_nic() {
  IP_CMD="{{.Commands.IP}}"
  IP_VERSION="$1"

  # Expected format: "default via <GATEWAY_IP> dev <PRIMARY_NIC> ..."
  # (there could be more tokens, but we just need the 3rd and 5th).
  $IP_CMD "$IP_VERSION" route list exact default | cut -d' ' -f3,5

  return 0
}
`
const dumpNetworkConfigScript = `
# Display route table contents (IPv4 and IPv6), network devices,
# all configured IPv4 and IPv6 addresses, and the contents
# of {{.Config}} for diagnosing connectivity issues.
dump_network_config() {
  IP_CMD="{{.Commands.IP}}"
  IFCONFIG_CMD="{{.Commands.IfConfig}}"

  echo "Current networking configuration:"
  echo "-------------------------------------------------------"

  echo "Route table contents:"
  # Using -B here shows both IPv4 and IPv6 routes.
  $IP_CMD -B route show
  echo "-------------------------------------------------------"

  echo "Network devices:"
  # Using 'ifconfig -a' instead of 'ip -B link show' formats better.
  $IFCONFIG_CMD -a
  echo "-------------------------------------------------------"

  echo "Configured IPv4 addresses:"
  $IP_CMD -4 address show
  echo "-------------------------------------------------------"

  echo "Configured IPv6 addresses:"
  $IP_CMD -6 address show
  echo "-------------------------------------------------------"

  echo "Contents of {{.Config}}:"
  cat {{.Config}}
  printf "\n%s\n" "-------------------------------------------------------"

  return 0
}
`

const modifyNetworkConfigScript = `
# Used by setup_bridge_config to do the actual modifications to
# {{.Config}}, based on the discovered $PRIMARY_NIC and $AF_NAME.
# No changes will be made unless all of following are true:
# 1. Both arguments are non-empty.
# 2. First argument is "inet" or "inet6".
# 3. Second argument matches the regexp [a-zA-Z0-9_:.-]+
# 4. A stanza "iface $PRIMARY_NIC $AF_NAME" exists.
# 5. A stanza "auto $PRIMARY_NIC" exists.
# Non-zero return code indicates no modifications were made,
# due to not meeting one or more of the above conditions. Zero
# return means all modifications were completed successfully.
# Arguments:
#  $1: address family (inet or inet6)
#  $2: discovered primary NIC
modify_network_config() {
  AF_NAME="$1"
  PRIMARY_NIC="$2"

  if [ -z "$AF_NAME" ] || [ -z "$PRIMARY_NIC" ]; then
    return 1
  fi
  if [ "$AF_NAME" != "inet" ] && [ "$AF_NAME" != "inet6" ]; then
    return 1
  fi
  # Ensure $PRIMARY_NIC contains only valid characters.
  printf "%q" "$PRIMARY_NIC" | tr -cs 'a-zA-Z0-9_:.-' '=' | grep -q '=' && return 1

  grep -qe 'iface $PRIMARY_NIC\s+$AF_NAME\s+" "{{.Config}}" || return 1
  grep -qe 'auto $PRIMARY_NIC[^:]*$" "{{.Config}}" || return 1

  sed -ri "s/^\s*iface\s+${PRIMARY_NIC}\s+${AF_NAME}\s+(.*)$/iface {{.Bridge}} $AF_NAME \1/" "{{.Config}}"
  sed -ri "s/^\s*auto\s+${PRIMARY_NIC}\s*$/auto {{.Bridge}}/" "{{.Config}}"
  sed -i "/iface {{.Bridge}} ${AF_NAME} /a\    bridge_ports ${PRIMARY_NIC}" "{{.Config}}"
  sed -i "/auto {{.Bridge}}/i\iface ${PRIMARY_NIC} ${AF_NAME} manual\n" "{{.Config}}"
  # Any existing aliases of the primary NIC (e.g. like eth0:0, eth0:1) must also
  # be moved over as aliases of {{.Bridge}}, otherwise they'll stop working.
  sed -ri "s/^\s*auto\s+${PRIMARY_NIC}(:.+)\s*$/auto {{.Bridge}}\1/" "{{.Config}}"
  sed -ri "s/^\s*iface\s+${PRIMARY_NIC}(:.+)\s+${AF_NAME}\s+(.*)$/iface {{.Bridge}}\1 $AF_NAME \2/" "{{.Config}}"

  return 0
}
`

const revertNetworkConfigScript = `
# Make a best effort to restore the changes made to
# {{.Config}}, delete {{.Bridge}} if it got created,
# and bring up all original network interfaces.
revert_network_config() {
  IP_CMD="{{.Commands.IP}}"
  IFUP_CMD="{{.Commands.IfUp}}"

  # Any of the following commands can fail, hence || true at the end.

  echo "Removing {{.Bridge}}, if it got created."
  $IP_CMD link del dev "{{.Bridge}}" || true

  if [ -f "{{.Config}}.original" ]; then
    echo "Modified config saved to {{.Config}}.juju"
    cp -f "{{.Config}}" "{{.Config}}.juju" || true

    echo "Reverting changes to {{.Config}} to restore connectivity."
    mv -f "{{.Config}}.original" "{{.Config}}" || true
  fi

  echo "Bringing up all previously configured interfaces."
  # First, try without --force, so the ifquery.state gets updated
  # properly, falling back to --force if it fails.
  $IFUP_CMD -a -v || $IFUP_CMD -a -v --force || true

  return 0
}
`

const setupBridgeConfigScript = `
# Make the following changes to {{.Config}} and brings up {{.Bridge}}:
# 1. Replaces $PRIMARY_NIC's name with {{.Bridge}} ("auto" and "iface" stanzas).
# 2. Adds "bridge_ports $PRIMARY_NIC" to the section for {{.Bridge}}.
# 3. Adds "iface $PRIMARY_NIC $AF_NAME manual" before the same section.
# 4. Any existing aliases of the primary NIC are moved over to the bridge.
# The original {{.Config}} is saved as {{.Config}}.original.
# Finally, the {{.Bridge}} device is added and activated. If this fails,
# the modifications are retained in {{.Config}}.juju and the original config
# is restored to ensure connectivity (for debugging) is preserved.
# Arguments:
#   $1: address family to use (e.g. "inet" for IPv4, "inet6" for IPv6)
#   $2: primary NIC device name (as discovered from the default route)
# On failure, returns a non-zero exit code.
setup_bridge_config() {
  AF_NAME="$1"
  PRIMARY_NIC="$2"
  IFUP_CMD="{{.Commands.IfUp}}"
  IFDOWN_CMD="{{.Commands.IfDown}}"
  IP_CMD="{{.Commands.IP}}"

  if [ "$AF_NAME" != "inet" ] && [ "$AF_NAME" != "inet6" ]; then
    echo "ERROR: address family $AF_NAME not supported: expected inet or inet6!"
    return 1
  fi
  if [ -z "$PRIMARY_NIC" ]; then
    echo "ERROR: primary NIC cannot be empty!"
    return 1
  fi
  if [ ! -f "{{.Config}}" ]; then
    echo "ERROR: {{.Config}} not found - cannot modify!"
    return 1
  fi

  echo "Bringing $PRIMARY_NIC down".
  # Bring down the primary interface while {{.Config}}
  # still matches the live config. Will bring it back up within a
  # bridge after updating {{.Config}}
  $IFDOWN_CMD -v "$PRIMARY_NIC"

  echo "Modifying {{.Config}} to replace $PRIMARY_NIC with {{.Bridge}} for AF $AF_NAME"
  echo "Saving original to {{.Config}}.original"
  cp -f "{{.Config}}" "{{.Config}}.original"

  modify_network_config "$AF_NAME" "$PRIMARY_NIC"
  if [ "$?" != 0 ]; then
    echo "ERROR: Unexpected contents of {{.Config}} (not modified)."
    echo "Bringing $PRIMARY_NIC up again."
    $IFUP_CMD -v "$PRIMARY_NIC"
    return 1
  fi

  echo "{{.Config}} updated successfully!"

  $IP_CMD link add dev "$PRIMARY_NIC" name "{{.Bridge}}" type bridge
  if [ "$?" != "0" ]; then
    echo "ERROR: cannot add {{.Bridge}} bridge!"
    return 1
  else
    echo "{{.Bridge}} created successfully, bringing it up."

    # NOTE: We need to bring up the bridge and any possible
    # new aliases it "acquired" from the primary NIC, so the
    # easiest way is just $IFUP_CMD -a, which takes care of setting
    # the addresses and routes as well.
    $IFUP_CMD -a -v
    if [ "$?" != "0" ]; then
      echo "ERROR: cannot bring {{.Bridge}} device up!"
      return 1
    fi
  fi

  echo "{{.Bridge}} activated, about to verify connectivity."
  return 0
}
`

const ensureBridgeConnectivityScript = `
# Because the bridge can take some time to come up, waiting for
# the primary NIC to enter forwarding state, wait up to 60s, pinging
# the default gateway via {{.Bridge}} each second, bailing out early
# on success. This ensures the tools downloading (happening soon after
# this script) will work.
# Arguments:
#   $1: default gateway (IPv4/IPv6) address to ping
#   $2: ping command to use (e.g. "ping" or "ping6")
# On failure, returns a non-zero exit code.
ensure_bridge_connectivity() {
  DEFAULT_GATEWAY="$1"
  PING_CMD="$(dirname {{.Commands.Ping}})/$2"
  IP_CMD="{{.Commands.IP}}"
  IFUP_CMD="{{.Commands.IfUp}}"

  # Find out the first of the primary, globally-scoped addresses
  # for {{.Bridge}} to use it in $PING_CMD below (using -I <IP>
  # rather than -I <NIC> works better for both IPv4 and IPv6).
  BRIDGE_ADDRS="$($IP_CMD -o address show dev {{.Bridge}} primary scope global | head -n1)"
  FIRST_BRIDGE_IP=$(echo "$BRIDGE_ADDRS" | sed -r "s/.*inet6? ([^\/]+)\/.*/\1/")
  TOTAL_TIMEOUT=60
  FAILURES_SO_FAR=0
  echo "Waiting up to ${TOTAL_TIMEOUT}s for {{.Bridge}} to successfully ping $DEFAULT_GATEWAY from $FIRST_BRIDGE_IP"
  for ATTEMPT in $(seq 1 $TOTAL_TIMEOUT);
  do
    # We ping once for one second, as we expect the default gateway
    # to be at most a single hop away and accessible in less than 1s.
    $PING_CMD -q -c 1 -w 1 -I "$FIRST_BRIDGE_IP" "$DEFAULT_GATEWAY" > /dev/null
    PING_RESULT="$?"
    case $PING_RESULT in
      0) # Success!
         echo "{{.Bridge}} can access $DEFAULT_GATEWAY successfully!"
         return 0
         ;;
      2) # Unexpected error (e.g. bad arguments, etc.)
         # Turn on verbose logging for easier debugging.
         set -x
         continue
         ;;
    esac

    # Don't spam the logs with too many failures, only every 10 failures.
    FAILURES_SO_FAR=$((FAILURES_SO_FAR+1))
    if [ $FAILURES_SO_FAR -ge 10 ]; then
        echo "{{.Bridge}} cannot access $DEFAULT_GATEWAY (retrying: attempt $ATTEMPT of $TOTAL_TIMEOUT)"
        FAILURES_SO_FAR=0
    fi
  done

  echo "ERROR: {{.Bridge}} cannot access $DEFAULT_GATEWAY (giving up after ${TOTAL_TIMEOUT}s)!"
  return 1
}
`

const bridgeConfigForIPVersionScript = `
# Discovers the primary NIC and gateway and modifies 
# {{.Config}} as needed, depending on the IP version,
# and finally ensures connectivity via {{.Bridge}} works.
# In case it fails, reverts back to the original config.
# Arguments:
#   $1: IP config from get_gateway_and_primary_nic
#   $2: IP version label (e.g. IPv4 or IPV6).
#   $3: Address family for the IP version (e.g. inet or inet6)
#   $4: Ping command variant to use (e.g. ping or ping6), no path.
# On failure, returns a non-zero exit code.
bridge_config_for_ip_version() {
  IP_CONFIG="$1"
  IP_VERSION="$2"
  AF_NAME="$3"
  PING_VARIANT="$4"

  echo "Configuring {{.Bridge}} for $IP_VERSION"

  DEFAULT_GATEWAY=$(echo "$IP_CONFIG" | cut -d' ' -f1)
  echo "Default gateway: $DEFAULT_GATEWAY"
  PRIMARY_NIC=$(echo "$IP_CONFIG" | cut -d' ' -f2)
  echo "Primary NIC: $PRIMARY_NIC"

  setup_bridge_config "$AF_NAME" "$PRIMARY_NIC" || return 1
  ensure_bridge_connectivity "$DEFAULT_GATEWAY" "$PING_VARIANT" || return 1

  return 0
}
`

const mainBridgeConfigTemplate = `
# In case we already created the bridge, don't do it again.
grep -q "iface {{.Bridge}} inet" "{{.Config}}" && exit 0

# Minimize the debugging output of this script (turned on for runcmds
# in cloud-init userdata by default), as it's already too verbose.
# Debug logging is re-enabled before exiting this script.
set +x

{{.Scripts.get_gateway_and_primary_nic}}
{{.Scripts.dump_network_config}}
{{.Scripts.modify_network_config}}
{{.Scripts.revert_network_config}}
{{.Scripts.setup_bridge_config}}
{{.Scripts.ensure_bridge_connectivity}}
{{.Scripts.bridge_config_for_ip_version}}

# Determine whether to configure {{.Bridge}} for IPv4, IPv6, or both.
IPV4_CONFIG="$(get_gateway_and_primary_nic -4)"
IPV6_CONFIG="$(get_gateway_and_primary_nic -6)"

# If this script returns exit code 1, the configuration
# failed and a best effort was made to restore it.

if [ -z "$IPV4_CONFIG" ] && [ -z "$IPV6_CONFIG" ]; then
  echo "FATAL: Cannot discover neither IPv4 nor IPv6 config for {{.Bridge}}!"
  dump_network_config
  set -x
  exit 1
fi

IPV6_CONFIG_RESULT=0
if [ -n "$IPV6_CONFIG" ]; then
  bridge_config_for_ip_version "$IPV6_CONFIG" "IPv6" "inet6" "ping6"
  IPV6_CONFIG_RESULT="$?"
fi

IPV4_CONFIG_RESULT=0
if [ -n "$IPV4_CONFIG" ]; then
  bridge_config_for_ip_version "$IPV4_CONFIG" "IPv4" "inet" "ping"
  IPV4_CONFIG_RESULT="$?"
fi

if [ "$IPV6_CONFIG_RESULT" != "0" ] || [ "$IPV4_CONFIG_RESULT" != "0" ]; then
  echo "FATAL: {{.Bridge}} not configured successfully."
  dump_network_config
  revert_network_config
  set -x
  exit 1
fi

echo "{{.Bridge}} for LXC/KVM machines configured and working!"
dump_network_config
set -x
`

// setupJujuNetworking returns a string representing the script to run
// in order to prepare the Juju-specific networking config on a node.
func setupJujuNetworking() (string, error) {
	args, err := prepareScriptsAndArgs(
		"/etc/network/interfaces",
		instancecfg.DefaultBridgeName,
		"/sbin",
		"/bin",
	)
	if err != nil {
		return "", errors.Annotate(err, "preparing bridge config script failed")
	}
	rendered, err := renderScript(args, "mainBridgeConfig", mainBridgeConfigTemplate)
	if err != nil {
		return "", errors.Annotate(err, "cannot render bridge config script")
	}
	return rendered, nil
}

func prepareScriptsAndArgs(configPath, bridgeName, sbinPath, binPath string) (scriptArgs, error) {
	args := scriptArgs{
		Config: configPath,
		Bridge: bridgeName,
		Commands: map[string]string{
			"IP":       filepath.Join(sbinPath, "ip"),
			"Ping":     filepath.Join(binPath, "ping"),
			"IfConfig": filepath.Join(sbinPath, "ifconfig"),
			"IfUp":     filepath.Join(sbinPath, "ifup"),
			"IfDown":   filepath.Join(sbinPath, "ifdown"),
		},
		Scripts: map[string]string{
			"get_gateway_and_primary_nic":  getGatewayAndPrimaryNICScript,
			"dump_network_config":          dumpNetworkConfigScript,
			"modify_network_config":        modifyNetworkConfigScript,
			"revert_network_config":        revertNetworkConfigScript,
			"setup_bridge_config":          setupBridgeConfigScript,
			"ensure_bridge_connectivity":   ensureBridgeConnectivityScript,
			"bridge_config_for_ip_version": bridgeConfigForIPVersionScript,
		},
	}

	for name, template := range args.Scripts {
		rendered, err := renderScript(args, name, template)
		if err != nil {
			return args, errors.Trace(err)
		}
		args.Scripts[name] = rendered
	}
	return args, nil
}

func renderScript(args scriptArgs, name, content string) (string, error) {
	parsed, err := template.New(name).Parse(content)
	if err != nil {
		return "", errors.Annotatef(err, "parsing template script %q", name)
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, args); err != nil {
		return "", errors.Annotatef(err, "rendering script %q", name)
	}
	return buf.String(), nil
}
