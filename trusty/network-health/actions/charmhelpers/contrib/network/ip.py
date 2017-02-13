# Copyright 2014-2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import glob
import re
import subprocess
import six
import socket

from functools import partial

from charmhelpers.core.hookenv import unit_get
from charmhelpers.fetch import apt_install, apt_update
from charmhelpers.core.hookenv import (
    log,
    WARNING,
)

try:
    import netifaces
except ImportError:
    apt_update(fatal=True)
    apt_install('python-netifaces', fatal=True)
    import netifaces

try:
    import netaddr
except ImportError:
    apt_update(fatal=True)
    apt_install('python-netaddr', fatal=True)
    import netaddr


def _validate_cidr(network):
    try:
        netaddr.IPNetwork(network)
    except (netaddr.core.AddrFormatError, ValueError):
        raise ValueError("Network (%s) is not in CIDR presentation format" %
                         network)


def no_ip_found_error_out(network):
    errmsg = ("No IP address found in network(s): %s" % network)
    raise ValueError(errmsg)


def get_address_in_network(network, fallback=None, fatal=False):
    """Get an IPv4 or IPv6 address within the network from the host.

    :param network (str): CIDR presentation format. For example,
        '192.168.1.0/24'. Supports multiple networks as a space-delimited list.
    :param fallback (str): If no address is found, return fallback.
    :param fatal (boolean): If no address is found, fallback is not
        set and fatal is True then exit(1).
    """
    if network is None:
        if fallback is not None:
            return fallback

        if fatal:
            no_ip_found_error_out(network)
        else:
            return None

    networks = network.split() or [network]
    for network in networks:
        _validate_cidr(network)
        network = netaddr.IPNetwork(network)
        for iface in netifaces.interfaces():
            addresses = netifaces.ifaddresses(iface)
            if network.version == 4 and netifaces.AF_INET in addresses:
                addr = addresses[netifaces.AF_INET][0]['addr']
                netmask = addresses[netifaces.AF_INET][0]['netmask']
                cidr = netaddr.IPNetwork("%s/%s" % (addr, netmask))
                if cidr in network:
                    return str(cidr.ip)

            if network.version == 6 and netifaces.AF_INET6 in addresses:
                for addr in addresses[netifaces.AF_INET6]:
                    if not addr['addr'].startswith('fe80'):
                        cidr = netaddr.IPNetwork("%s/%s" % (addr['addr'],
                                                            addr['netmask']))
                        if cidr in network:
                            return str(cidr.ip)

    if fallback is not None:
        return fallback

    if fatal:
        no_ip_found_error_out(network)

    return None


def is_ipv6(address):
    """Determine whether provided address is IPv6 or not."""
    try:
        address = netaddr.IPAddress(address)
    except netaddr.AddrFormatError:
        # probably a hostname - so not an address at all!
        return False

    return address.version == 6


def is_address_in_network(network, address):
    """
    Determine whether the provided address is within a network range.

    :param network (str): CIDR presentation format. For example,
        '192.168.1.0/24'.
    :param address: An individual IPv4 or IPv6 address without a net
        mask or subnet prefix. For example, '192.168.1.1'.
    :returns boolean: Flag indicating whether address is in network.
    """
    try:
        network = netaddr.IPNetwork(network)
    except (netaddr.core.AddrFormatError, ValueError):
        raise ValueError("Network (%s) is not in CIDR presentation format" %
                         network)

    try:
        address = netaddr.IPAddress(address)
    except (netaddr.core.AddrFormatError, ValueError):
        raise ValueError("Address (%s) is not in correct presentation format" %
                         address)

    if address in network:
        return True
    else:
        return False


def _get_for_address(address, key):
    """Retrieve an attribute of or the physical interface that
    the IP address provided could be bound to.

    :param address (str): An individual IPv4 or IPv6 address without a net
        mask or subnet prefix. For example, '192.168.1.1'.
    :param key: 'iface' for the physical interface name or an attribute
        of the configured interface, for example 'netmask'.
    :returns str: Requested attribute or None if address is not bindable.
    """
    address = netaddr.IPAddress(address)
    for iface in netifaces.interfaces():
        addresses = netifaces.ifaddresses(iface)
        if address.version == 4 and netifaces.AF_INET in addresses:
            addr = addresses[netifaces.AF_INET][0]['addr']
            netmask = addresses[netifaces.AF_INET][0]['netmask']
            network = netaddr.IPNetwork("%s/%s" % (addr, netmask))
            cidr = network.cidr
            if address in cidr:
                if key == 'iface':
                    return iface
                else:
                    return addresses[netifaces.AF_INET][0][key]

        if address.version == 6 and netifaces.AF_INET6 in addresses:
            for addr in addresses[netifaces.AF_INET6]:
                if not addr['addr'].startswith('fe80'):
                    network = netaddr.IPNetwork("%s/%s" % (addr['addr'],
                                                           addr['netmask']))
                    cidr = network.cidr
                    if address in cidr:
                        if key == 'iface':
                            return iface
                        elif key == 'netmask' and cidr:
                            return str(cidr).split('/')[1]
                        else:
                            return addr[key]

    return None


get_iface_for_address = partial(_get_for_address, key='iface')


get_netmask_for_address = partial(_get_for_address, key='netmask')


def resolve_network_cidr(ip_address):
    '''
    Resolves the full address cidr of an ip_address based on
    configured network interfaces
    '''
    netmask = get_netmask_for_address(ip_address)
    return str(netaddr.IPNetwork("%s/%s" % (ip_address, netmask)).cidr)


def format_ipv6_addr(address):
    """If address is IPv6, wrap it in '[]' otherwise return None.

    This is required by most configuration files when specifying IPv6
    addresses.
    """
    if is_ipv6(address):
        return "[%s]" % address

    return None


def get_iface_addr(iface='eth0', inet_type='AF_INET', inc_aliases=False,
                   fatal=True, exc_list=None):
    """Return the assigned IP address for a given interface, if any.

    :param iface: network interface on which address(es) are expected to
                  be found.
    :param inet_type: inet address family
    :param inc_aliases: include alias interfaces in search
    :param fatal: if True, raise exception if address not found
    :param exc_list: list of addresses to ignore
    :return: list of ip addresses
    """
    # Extract nic if passed /dev/ethX
    if '/' in iface:
        iface = iface.split('/')[-1]

    if not exc_list:
        exc_list = []

    try:
        inet_num = getattr(netifaces, inet_type)
    except AttributeError:
        raise Exception("Unknown inet type '%s'" % str(inet_type))

    interfaces = netifaces.interfaces()
    if inc_aliases:
        ifaces = []
        for _iface in interfaces:
            if iface == _iface or _iface.split(':')[0] == iface:
                ifaces.append(_iface)

        if fatal and not ifaces:
            raise Exception("Invalid interface '%s'" % iface)

        ifaces.sort()
    else:
        if iface not in interfaces:
            if fatal:
                raise Exception("Interface '%s' not found " % (iface))
            else:
                return []

        else:
            ifaces = [iface]

    addresses = []
    for netiface in ifaces:
        net_info = netifaces.ifaddresses(netiface)
        if inet_num in net_info:
            for entry in net_info[inet_num]:
                if 'addr' in entry and entry['addr'] not in exc_list:
                    addresses.append(entry['addr'])

    if fatal and not addresses:
        raise Exception("Interface '%s' doesn't have any %s addresses." %
                        (iface, inet_type))

    return sorted(addresses)


get_ipv4_addr = partial(get_iface_addr, inet_type='AF_INET')


def get_iface_from_addr(addr):
    """Work out on which interface the provided address is configured."""
    for iface in netifaces.interfaces():
        addresses = netifaces.ifaddresses(iface)
        for inet_type in addresses:
            for _addr in addresses[inet_type]:
                _addr = _addr['addr']
                # link local
                ll_key = re.compile("(.+)%.*")
                raw = re.match(ll_key, _addr)
                if raw:
                    _addr = raw.group(1)

                if _addr == addr:
                    log("Address '%s' is configured on iface '%s'" %
                        (addr, iface))
                    return iface

    msg = "Unable to infer net iface on which '%s' is configured" % (addr)
    raise Exception(msg)


def sniff_iface(f):
    """Ensure decorated function is called with a value for iface.

    If no iface provided, inject net iface inferred from unit private address.
    """
    def iface_sniffer(*args, **kwargs):
        if not kwargs.get('iface', None):
            kwargs['iface'] = get_iface_from_addr(unit_get('private-address'))

        return f(*args, **kwargs)

    return iface_sniffer


@sniff_iface
def get_ipv6_addr(iface=None, inc_aliases=False, fatal=True, exc_list=None,
                  dynamic_only=True):
    """Get assigned IPv6 address for a given interface.

    Returns list of addresses found. If no address found, returns empty list.

    If iface is None, we infer the current primary interface by doing a reverse
    lookup on the unit private-address.

    We currently only support scope global IPv6 addresses i.e. non-temporary
    addresses. If no global IPv6 address is found, return the first one found
    in the ipv6 address list.

    :param iface: network interface on which ipv6 address(es) are expected to
                  be found.
    :param inc_aliases: include alias interfaces in search
    :param fatal: if True, raise exception if address not found
    :param exc_list: list of addresses to ignore
    :param dynamic_only: only recognise dynamic addresses
    :return: list of ipv6 addresses
    """
    addresses = get_iface_addr(iface=iface, inet_type='AF_INET6',
                               inc_aliases=inc_aliases, fatal=fatal,
                               exc_list=exc_list)

    if addresses:
        global_addrs = []
        for addr in addresses:
            key_scope_link_local = re.compile("^fe80::..(.+)%(.+)")
            m = re.match(key_scope_link_local, addr)
            if m:
                eui_64_mac = m.group(1)
                iface = m.group(2)
            else:
                global_addrs.append(addr)

        if global_addrs:
            # Make sure any found global addresses are not temporary
            cmd = ['ip', 'addr', 'show', iface]
            out = subprocess.check_output(cmd).decode('UTF-8')
            if dynamic_only:
                key = re.compile("inet6 (.+)/[0-9]+ scope global.* dynamic.*")
            else:
                key = re.compile("inet6 (.+)/[0-9]+ scope global.*")

            addrs = []
            for line in out.split('\n'):
                line = line.strip()
                m = re.match(key, line)
                if m and 'temporary' not in line:
                    # Return the first valid address we find
                    for addr in global_addrs:
                        if m.group(1) == addr:
                            if not dynamic_only or \
                                    m.group(1).endswith(eui_64_mac):
                                addrs.append(addr)

            if addrs:
                return addrs

    if fatal:
        raise Exception("Interface '%s' does not have a scope global "
                        "non-temporary ipv6 address." % iface)

    return []


def get_bridges(vnic_dir='/sys/devices/virtual/net'):
    """Return a list of bridges on the system."""
    b_regex = "%s/*/bridge" % vnic_dir
    return [x.replace(vnic_dir, '').split('/')[1] for x in glob.glob(b_regex)]


def get_bridge_nics(bridge, vnic_dir='/sys/devices/virtual/net'):
    """Return a list of nics comprising a given bridge on the system."""
    brif_regex = "%s/%s/brif/*" % (vnic_dir, bridge)
    return [x.split('/')[-1] for x in glob.glob(brif_regex)]


def is_bridge_member(nic):
    """Check if a given nic is a member of a bridge."""
    for bridge in get_bridges():
        if nic in get_bridge_nics(bridge):
            return True

    return False


def is_ip(address):
    """
    Returns True if address is a valid IP address.
    """
    try:
        # Test to see if already an IPv4/IPv6 address
        address = netaddr.IPAddress(address)
        return True
    except (netaddr.AddrFormatError, ValueError):
        return False


def ns_query(address):
    try:
        import dns.resolver
    except ImportError:
        apt_install('python-dnspython', fatal=True)
        import dns.resolver

    if isinstance(address, dns.name.Name):
        rtype = 'PTR'
    elif isinstance(address, six.string_types):
        rtype = 'A'
    else:
        return None

    answers = dns.resolver.query(address, rtype)
    if answers:
        return str(answers[0])
    return None


def get_host_ip(hostname, fallback=None):
    """
    Resolves the IP for a given hostname, or returns
    the input if it is already an IP.
    """
    if is_ip(hostname):
        return hostname

    ip_addr = ns_query(hostname)
    if not ip_addr:
        try:
            ip_addr = socket.gethostbyname(hostname)
        except:
            log("Failed to resolve hostname '%s'" % (hostname),
                level=WARNING)
            return fallback
    return ip_addr


def get_hostname(address, fqdn=True):
    """
    Resolves hostname for given IP, or returns the input
    if it is already a hostname.
    """
    if is_ip(address):
        try:
            import dns.reversename
        except ImportError:
            apt_install("python-dnspython", fatal=True)
            import dns.reversename

        rev = dns.reversename.from_address(address)
        result = ns_query(rev)

        if not result:
            try:
                result = socket.gethostbyaddr(address)[0]
            except:
                return None
    else:
        result = address

    if fqdn:
        # strip trailing .
        if result.endswith('.'):
            return result[:-1]
        else:
            return result
    else:
        return result.split('.')[0]


def port_has_listener(address, port):
    """
    Returns True if the address:port is open and being listened to,
    else False.

    @param address: an IP address or hostname
    @param port: integer port

    Note calls 'zc' via a subprocess shell
    """
    cmd = ['nc', '-z', address, str(port)]
    result = subprocess.call(cmd)
    return not(bool(result))
