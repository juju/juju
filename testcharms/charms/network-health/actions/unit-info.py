#!/usr/local/sbin/charm-env python3
import subprocess
import ipaddress
import re

from charmhelpers.core.hookenv import (
    action_set
)


def main():
    unit_interfaces = interfaces()
    action_set({'interfaces': unit_interfaces})


def interfaces():
    if_matches = re.findall(
        (r'\d+: (?P<ifname>[^:@]+).*?link/[^\s]+ (?P<hwaddr>[^\s]+)'
         r'.*?RX:.*?(?P<rx>[\d]+).*?TX:.*?(?P<tx>[\d]+)'),
        subprocess.check_output('ip -s link', shell=True).decode("utf-8"),
        re.DOTALL
    )

    all_ifs = {}
    for if_match in if_matches:
        name, hwaddr, rx, tx = if_match
        iface = {
            'device': name,
            'ether': hwaddr,
            'rxbytes': rx,
            'txbytes': tx,
        }

        # Grab address details
        addr_out = subprocess.check_output('ip addr show ' + name,
                                           shell=True).decode("utf-8")

        inet_patterns = [
            # address with no broadcast (e.g. if this is a loopback device)
            r'.*inet (?P<addr>[^\s]+) scope (?P<scope>[^\s]*).*',
            # address with broadcast
            (r'.*inet (?P<addr>[^\s]+) brd (?P<broadcast>[^\s]*).*?'
             r' scope (?P<scope>[^\s]*).*'),
        ]
        for pat in inet_patterns:
            match = re.search(pat, addr_out)
            if not match:
                continue

            match_dict = match.groupdict()
            ipnet = ipaddress.ip_interface(match_dict['addr'])
            iface['inet_addr'] = format(ipnet.ip)
            iface['inet_netmask'] = format(ipnet.netmask)
            iface['scope'] = match_dict['scope']
            if 'broadcast' in match_dict:
                iface['inet_broadcast'] = match_dict['broadcast']

        inet6_patterns = [
            # address with no broadcast (e.g. if this is a loopback device)
            r'.*inet6 (?P<addr>[^\s]+) scope (?P<scope>[^\s]*).*',
            # address with broadcast
            (r'.*inet6 (?P<addr>[^\s]+) brd (?P<broadcast>[^\s]*).*?'
             r' scope (?P<scope>[^\s]*).*'),
        ]
        for pat in inet6_patterns:
            match = re.search(pat, addr_out)
            if not match:
                continue

            match_dict = match.groupdict()
            ipnet = ipaddress.ip_interface(match_dict['addr'])
            iface['inet6_addr'] = format(ipnet.ip)
            iface['inet6_netmask'] = format(ipnet.netmask)
            iface['scope'] = match_dict['scope']
            if 'broadcast' in match_dict:
                iface['inet6_broadcast'] = match_dict['broadcast']

        all_ifs[name] = iface
    return all_ifs


if __name__ == "__main__":
    main()
