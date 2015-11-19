// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

const bridgeScriptBase = `
# Print message with function and line number info from perspective of
# the caller and exit with status code 1.
fatal()
{
    local message=$1
    echo "${BASH_SOURCE[1]}: line ${BASH_LINENO[0]}: ${FUNCNAME[1]}: fatal error: ${message:-'died'}." >&2
    exit 1
}

# Modifies $configfile to enslave $primary_nic using $bridge.
#
# This function does not ifdown/up any interfaces, it merely rewrites
# $configfile so that this can be done as and when necessary.
#
# Returns 1 on any error, otherwise 0 for success.
#
modify_network_config() {
    [ $# -lt 4 ] && return 1

    if [ -z "$1" ] || [ -z "$2" ] || [ -z "$3" ] || [ -z "$4" ]; then
	return 1
    fi

    local address_family=$1
    local primary_nic=$2
    local configfile=$3
    local bridge=$4

    [ -f "$configfile" ] || return 1

    if [ "$address_family" != "inet" ] && [ "$address_family" != "inet6" ]; then
	return 1
    fi

    grep -q -E "iface $primary_nic\s+$address_family\s+" "$configfile" || return 1
    grep -q -E "auto $primary_nic[^:]*$" "$configfile" || return 1

    # Change:
    #     iface eth0 inet dhcp|manual|static
    # to:
    #     iface juju-br0 inet dhcp|manual|static
    sed -ri "s/^\s*iface\s+${primary_nic}\s+${address_family}\s+(.*)$/iface $bridge $address_family \1/" "$configfile" || fatal 'sed failed'

    # Change:
    #     auto eth0
    # to:
    #     auto juju-br0
    sed -ri "s/^\s*auto\s+${primary_nic}\s*$/auto $bridge/" "$configfile" || fatal 'sed failed'

    # Append line after:
    #     iface juju-br0 inet
    # to:
    #     iface juju-br0 inet
    #         bridge_ports eth0
    #
    sed -i "/^iface $bridge $address_family /a\    bridge_ports $primary_nic" "$configfile" || fatal 'sed failed'

    # Ensure the existing primary nic becomes manual.
    # Change:
    #     auto juju-br0
    # to:
    #     iface eth0 inet manual
    #     auto juju-br0
    #
    sed -i "/^auto $bridge/iiface $primary_nic $address_family manual\n" "$configfile" || fatal 'sed failed'

    # Also enslave any aliases (e.g. like eth0:0, eth0:1).

    # Change:
    #     auto eth0:1
    #     iface eth0:1 inet static
    # to:
    #     auto juju-br0:1
    #     iface juju-br0:1 inet static
    sed -ri "s/^\s*auto\s+${primary_nic}(:.+)\s*$/auto $bridge\1/" "$configfile" || fatal 'sed failed'
    sed -ri "s/^\s*iface\s+${primary_nic}(:.+)\s+${address_family}\s+(.*)$/iface $bridge\1 $address_family \2/" "$configfile" || fatal 'sed failed'

    return 0
}

# Discover the needed IPv4/IPv6 configuration for $BRIDGE (if any).
#
# Arguments:
#
#   $1: the first argument to $IP_CMD (e.g. "-6" or "-4")
#
# Outputs the discovered default gateway and primary NIC, separated
# with a space, if they could be discovered. The output is undefined
# otherwise.
get_gateway() {
    $IP_CMD "$1" route list exact default | cut -d' ' -f3
}

get_primary_nic() {
    $IP_CMD "$1" route list exact default | cut -d' ' -f5
}

# Display route table contents (IPv4 and IPv6), network devices, all
# configured IPv4 and IPv6 addresses, and the contents of $CONFIGFILE
# for diagnosing connectivity issues.
dump_network_config() {
    # Note: Use the simplest command and options to be compatible with
    # precise.

    echo "======================================================="
    echo "${1} Network Configuration"
    echo "======================================================="
    echo

    echo "-------------------------------------------------------"
    echo "Route table contents:"
    echo "-------------------------------------------------------"
    $IP_CMD route show
    echo

    echo "-------------------------------------------------------"
    echo "Network devices:"
    echo "-------------------------------------------------------"
    $IFCONFIG_CMD -a
    echo

    echo "-------------------------------------------------------"
    echo "Contents of $CONFIGFILE"
    echo "-------------------------------------------------------"
    cat "$CONFIGFILE"
}
`

const bridgeScriptMain = `
: ${CONFIGFILE:={{.Config}}}
: ${PING_CMD:="ping"}
: ${IP_CMD:="ip"}
: ${IFUP_CMD:="ifup"}
: ${IFDOWN_CMD:="ifdown"}
: ${IFCONFIG_CMD:="ifconfig"}
: ${BRIDGE:={{.Bridge}}}

set -u

main() {
    local orig_config_file="$CONFIGFILE"
    local new_config_file="${CONFIGFILE}-juju"

    # In case we already created the bridge, don't do it again.
    grep -q "iface $BRIDGE inet" "$orig_config_file" && exit 0

    # We're going to do all our mods against a new file.
    cp -a "$CONFIGFILE" "$new_config_file" || fatal "cp failed"

    # Take a one-time reference of the original file
    if [ ! -f "${CONFIGFILE}-orig" ]; then
	cp -a "$CONFIGFILE" "${CONFIGFILE}-orig" || fatal "cp failed"
    fi

    # determine whether to configure $bridge for ipv4, ipv6, or both.
    local ipv4_gateway=$(get_gateway -4)
    local ipv4_primary_nic=$(get_primary_nic -4)
    local ipv6_gateway=$(get_gateway -6)
    local ipv6_primary_nic=$(get_primary_nic -6)

    echo "ipv4 gateway = $ipv4_gateway"
    echo "ipv4 primary nic = $ipv4_primary_nic"
    echo
    echo "ipv6 gateway = $ipv6_gateway"
    echo "ipv6 primary nic = $ipv6_primary_nic"

    if [ -z "$ipv4_gateway" ] && [ -z "$ipv6_gateway" ]; then
	fatal "cannot discover ipv4 and ipv6 gateway"
    fi

    local modify_network_config_failed=0

    if [ -n "$ipv4_gateway" ]; then
	modify_network_config "inet" "$ipv4_primary_nic" "$new_config_file" "$BRIDGE"
	if [ $? -ne 0 ]; then
	    modify_network_config_failed=1
	fi
    fi

    if [ -n "$ipv6_gateway" ]; then
	# TODO This should be similar to the IPv4 block above.
	# TODO Further work and testing required for IPv6 setups.
	echo "Cannot enslave $ipv6_primary_nic; IPv6 not supported in this script"
    fi

    if [ $modify_network_config_failed -eq 1 ]; then
	fatal "failed to add $BRIDGE to $orig_config_file"
    fi

    if ! ip link list "$BRIDGE"; then
	$IP_CMD link add dev "$ipv4_primary_nic" name "$BRIDGE" type bridge
	if [ $? -ne 0 ]; then
	    fatal "cannot add $BRIDGE bridge"
	fi
    fi

    local nics=""

    if [ -n "$ipv4_gateway" ]; then
	nics="${nics} $ipv4_primary_nic"
    fi

    if [ -n "$ipv6_gateway" ]; then
	# TODO Further work and testing required for IPv6 setups.
	:
    fi

    echo "--------------------------------------------------
    echo "Activating $BRIDGE configuration"
    echo "--------------------------------------------------
    cat "$new_config_file"

    for nic in $nics; do
	$IFDOWN_CMD -v -i "$orig_config_file" "$nic"
	if [ $? -ne 0 ]; then
	    fatal "failed to bring down $nic"
	fi
    done

    $IFUP_CMD -v -i "$new_config_file" "$BRIDGE"
    if [ $? -ne 0 ]; then
	fatal "failed to bring up $BRIDGE"
    fi

    # Bring up all aliases or bonds on the bridge.
    $IFUP_CMD -a -v -i "$new_config_file"
    if [ $? -ne 0 ]; then
	fatal "failed to bring up all interfaces"
    fi

    mv -f "$new_config_file" "$orig_config_file" || fatal "mv failed"
}

trap 'dump_network_config "Active"' EXIT
dump_network_config "Current"
main
`
