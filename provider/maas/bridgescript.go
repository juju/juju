// This file is auto generated. Edits will be lost.

package maas

//go:generate make -q

const bridgeScriptPythonBashDef = `python_script=$(cat <<'PYTHON_SCRIPT'
#!/usr/bin/env python

# Copyright 2015 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

from __future__ import print_function
import argparse
import os
import re
import shutil
import subprocess
import sys


# This script re-renders an interfaces(5) file to enslave the primary
# NIC with a bridge. It is aware of bond interfaces and aliases.

class SeekableIterator(object):
    """An iterator that supports relative seeking."""

    def __init__(self, iterable):
        self.iterable = iterable
        self.index = 0

    def __iter__(self):
        return self

    def next(self):  # Python 2
        try:
            value = self.iterable[self.index]
            self.index += 1
            return value
        except IndexError:
            raise StopIteration

    def __next__(self):  # Python 3
        return self.next()

    def seek(self, n, relative=False):
        if relative:
            self.index += n
        else:
            self.index = n
        if self.index < 0 or self.index >= len(self.iterable):
            raise IndexError


class Stanza(object):
    """Represents one stanza together with all its options."""

    def __init__(self, definition, options=None):
        if not options:
            options = []
        self._definition = definition
        self._options = options

    def is_physical_interface(self):
        return self._definition.startswith('auto ')

    def is_logical_interface(self):
        return self._definition.startswith('iface ')

    def options(self):
        return self._options

    def definition(self):
        return self._definition

    def interface_name(self):
        if self.is_physical_interface():
            return self._definition.split()[1]
        if self.is_logical_interface():
            return self._definition.split()[1]
        return None


class NetworkInterfaceParser(object):
    """Parse a network interface file into its set of stanzas."""

    @classmethod
    def is_stanza(cls, s):
        return re.match(r'^(iface|mapping|auto|allow-|source|dns-)', s)

    def __init__(self, filename):
        self._filename = filename
        self._stanzas = []
        with open(filename) as f:
            lines = f.readlines()
        line_iterator = SeekableIterator(lines)
        for line in line_iterator:
            if self.is_stanza(line):
                stanza = self._parse_stanza(line, line_iterator)
                self._stanzas.append(stanza)

    def _parse_stanza(self, stanza_line, iterable):
        stanza_options = []
        for line in iterable:
            line = line.strip()
            if line.startswith('#') or line == "":
                continue
            if self.is_stanza(line):
                iterable.seek(-1, True)
                break
            stanza_options.append(line)
        return Stanza(stanza_line.strip(), stanza_options)

    def stanzas(self):
        return [x for x in self._stanzas]


def print_stanza(s, stream=sys.stdout):
    print(s.definition(), file=stream)
    for o in s.options():
        print("   ", o, file=stream)


def print_stanzas(stanzas, stream=sys.stdout):
    n = len(stanzas)
    for i, s in enumerate(stanzas):
        print_stanza(s, stream)
        if s.is_logical_interface() and i + 1 < n:
            print(file=stream)


# Parses filename and returns a set of stanzas that have the existing
# primary NIC bridged.
def add_bridge(filename, bridge_name, primary_nic, bonded):
    stanzas = []

    for s in NetworkInterfaceParser(filename).stanzas():
        if not s.is_logical_interface() and not s.is_physical_interface():
            stanzas.append(s)
            continue

        if primary_nic != s.interface_name() and \
                        primary_nic not in s.interface_name():
            stanzas.append(s)
            continue

        if bonded:
            if s.is_physical_interface():
                stanzas.append(s)
            else:
                iface, orig_name, addr_family, method = s.definition().split()
                stanzas.append(Stanza("iface {} {} manual".format(orig_name, addr_family), s.options()))

                # new auto <bridge_name>
                stanzas.append(Stanza("auto {}".format(bridge_name)))

                # new iface <bridge_name> ...
                options = [x for x in s.options() if not x.startswith("bond")]
                options.insert(0, "bridge_ports {}".format(primary_nic))
                options.append("pre-up ip link add dev {} name {} type bridge || true".format(orig_name, bridge_name))
                stanzas.append(Stanza("iface {} {} {}".format(bridge_name, addr_family, method), options))
            continue

        if primary_nic == s.interface_name():
            if s.is_physical_interface():
                # The net change:
                #   auto eth0
                # to:
                #   auto <bridge_name>
                words = s.definition().split()
                words[1] = bridge_name
                stanzas.append(Stanza(" ".join(words)))
            else:
                # The net change is:
                #   auto eth0
                #   iface eth0 inet <config>
                # to:
                #   iface eth0 inet manual
                #
                #   auto <bridge_name>
                #   iface <bridge_name> inet <config>
                words = s.definition().split()
                words[3] = "manual"
                if len(stanzas) > 0:
                    last_stanza = stanzas.pop()
                else:
                    last_stanza = None
                stanzas.append(Stanza(" ".join(words)))
                if last_stanza:
                    stanzas.append(last_stanza)
                # Replace existing 'iface' line with new <bridge_name>
                words = s.definition().split()
                words[1] = bridge_name
                options = s.options()
                options.insert(0, "bridge_ports {}".format(primary_nic))
                stanzas.append(Stanza(" ".join(words), options))
            continue

        # Aliases, hence the 'eth0' in 'auto eth0:1'.

        if primary_nic in s.definition():
            definition = s.definition().replace(primary_nic, bridge_name)
            stanzas.append(Stanza(definition, s.options()))

    return stanzas


def shell_cmd(s):
    p = subprocess.Popen(s, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    out, err = p.communicate()
    return [out, err, p.returncode]


def print_shell_cmd(s, verbose=True, exitOnError=False):
    if verbose: print(s)
    out, err, retcode = shell_cmd(s)
    if out and len(out) > 0:
        print(out.rstrip(chr(10)))
    if err and len(err) > 0:
        print(err.rstrip(chr(10)))
    if exitOnError and retcode != 0:
        sys.exit(1)


def check_shell_cmd(s, verbose=False):
    if verbose: print(s)
    output = subprocess.check_output(s, shell=True, stderr=subprocess.STDOUT).strip().decode("utf-8")
    if verbose: print(output.rstrip('\n'))
    return output


def get_gateway(ver='-4'):
    return check_shell_cmd("ip {} route list exact default | head -n1 | cut -d' ' -f3".format(ver))


def get_primary_nic(ver='-4'):
    return check_shell_cmd("ip {} route list exact default | head -n1 | cut -d' ' -f5".format(ver))


def is_nic_bonded(name):
    out, err, retcode = shell_cmd("cat {}".format("/sys/class/net/bonding_masters"))
    return name in out


def is_bridged(name, filename):
    for s in NetworkInterfaceParser(filename).stanzas():
        if name in s.definition():
            return True
    return False


def link_is_up(name):
    out, err, retcode = shell_cmd('ip link show {} up'.format(name))
    return re.search(r'\s+{}:\s+.*\s+state\s+UP\s+'.format(name), out)


def ifup(name, retries=5):
    if retries < 1:
        retries = 1
    i = 1
    while i <= retries:
        print("link {} not up, attempt ({}/{})".format(name, i, retries))
        print_shell_cmd("ifdown -v -a")
        print_shell_cmd("ifup -v -a")
        if link_is_up(name):
            return True
        print_shell_cmd("ip link")
        i += 1
    return False


parser = argparse.ArgumentParser()

parser.add_argument('--filename',
                    help='filename to re-render',
                    type=str,
                    required=False,
                    default="/etc/network/interfaces")

parser.add_argument('--bridge-name',
                    help="bridge name",
                    type=str,
                    required=False,
                    default='juju-br0')

parser.add_argument('--primary-nic',
                    help="primary NIC name",
                    type=str,
                    required=False)

parser.add_argument('--primary-nic-is-bonded',
                    help="primary NIC is bonded",
                    action='store_true',
                    required=False)

parser.add_argument('--render-only',
                    help='render to stdout, no network restart',
                    action='store_true',
                    required=False)

parser.add_argument('--backup-filename',
                    help='backup filename',
                    type=str,
                    required=False)

args = parser.parse_args()

if is_bridged(args.bridge_name, args.filename):
    print("already bridged; nothing to do")
    sys.exit(0)

if not args.primary_nic:
    args.primary_nic = get_primary_nic()

if not args.primary_nic_is_bonded:
    args.primary_nic_is_bonded = is_nic_bonded(args.primary_nic)

bridged_stanzas = add_bridge(args.filename,
                             args.bridge_name,
                             args.primary_nic,
                             args.primary_nic_is_bonded)

if args.render_only:
    print_stanzas(bridged_stanzas)
    sys.exit(0)

if not get_gateway():
    print("no default gw; continue continue")
    sys.exit(1)

if args.backup_filename and not os.path.isfile(args.backup_filename):
    shutil.copy2(args.filename, args.backup_filename)

print("**** Original configuration")
print_shell_cmd("cat {}".format(args.filename))
print_shell_cmd("ifconfig -a")
print_shell_cmd("ip route show")
print_shell_cmd("ifdown -v -a")

print("**** Activating new configuration")

with open(args.filename, 'w') as f:
    print_stanzas(bridged_stanzas, f)
    f.close()

if not ifup(args.bridge_name):
    sys.exit(1)

print_shell_cmd("ifconfig -a")
print_shell_cmd("ip route show")
print_shell_cmd("brctl show")
PYTHON_SCRIPT
)`
