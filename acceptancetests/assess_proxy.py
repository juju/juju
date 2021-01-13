#!/usr/bin/env python3
"""Assess Juju under various proxy network conditions.

This test is dangerous to run on your own host. It will change the host's
network and can lock you out of your host. There are checks to ensure the
host matches expectations so that the network can be reset. While the test
is running, other processes on the host may be crippled.
"""

from __future__ import print_function

import argparse
import logging
import os
import re
import subprocess
import sys

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


log = logging.getLogger("assess_proxy")

ENVIRONMENT_FILE = '/etc/environment'

SCENARIO_BOTH = 'both-proxied'
SCENARIO_CLIENT = 'client-proxied'
SCENARIO_CONTROLLER = 'controller-proxied'

IPTABLES_FORWARD_PROXY = '-A FORWARD -i {} -p tcp --d port 3128 -j ACCEPT'
IPTABLES_BACKUP = '/etc/iptables.before-assess-proxy'
IPTABLES_BACKUP_BASH = """\
#!/bin/bash
set -eux
sudo iptables-save | sudo tee {iptables_backup}
""".format(iptables_backup=IPTABLES_BACKUP)

UFW_PROXY_COMMON_BASH = """\
#!/bin/bash
set -eux
sudo ufw allow in 22/tcp
sudo ufw allow out 22/tcp
sudo ufw allow out 53/udp
sudo ufw allow out 123/udp
sudo ufw allow out 3128/tcp
"""
UFW_PROXY_CLIENT_BASH = """\
#!/bin/bash
set -eux
sudo ufw deny out on {interface} to any
"""
UFW_PROXY_CONTROLLER_BASH = """\
#!/bin/bash
set -eux
sudo ufw allow out on {interface} to any port 3128
sudo ufw allow out on {interface} to any port 53
sudo ufw allow out on {interface} to any port 67
sudo iptables -A FORWARD -i {interface} -p tcp --dport 3128 -j ACCEPT
sudo iptables -D {original_forward_rule}
"""
UFW_ENABLE_COMMAND = ['sudo', 'ufw', '--force', 'enable']
UFW_RESET_COMMANDS = [
    ('sudo', 'iptables-restore', IPTABLES_BACKUP),
    ('sudo', 'ufw', '--force', 'reset'),
    ('sudo', 'ufw', '--force', 'disable'),
]


class UndefinedProxyError(Exception):
    """The current env or /etc/environment does not define proxy info."""


def get_environment_file_path():
    return ENVIRONMENT_FILE


def check_environment():
    """Verify the current env and etc/environment' define a proxy.

    Check for lowercase names because Juju uses them, some charms/apps might
    want uppercase names. This check assumes the env has both upper and lower
    case forms.

    :return: a tuple of http_proxy, https_proxy, ftp_proxy, no_proxy.
    """
    http_proxy = os.environ.get('http_proxy', None)
    https_proxy = os.environ.get('https_proxy', None)
    ftp_proxy = os.environ.get('ftp_proxy', None)
    no_proxy = os.environ.get('no_proxy', None)
    if http_proxy is None or https_proxy is None:
        message = 'http_proxy and https_proxy not defined in env'
        log.error(message)
        raise UndefinedProxyError(message)
    try:
        with open(get_environment_file_path(), 'r') as env_file:
            env_data = env_file.read()
    except IOError:
        env_data = ''
    found_http_proxy = False
    found_https_proxy = False
    for line in env_data.splitlines():
        if '=' in line:
            key, value = line.split('=', 1)
            if key == 'http_proxy' and value == http_proxy:
                found_http_proxy = True
            elif key == 'https_proxy' and value == https_proxy:
                found_https_proxy = True
    if not found_http_proxy or not found_https_proxy:
        message = (
            'http_proxy and https_proxy not defined in /etc/environment')
        log.error(message)
        raise UndefinedProxyError(message)
    log.info('Proxy env is:')
    log.info('http_proxy={}'.format(http_proxy))
    log.info('https_proxy={}'.format(https_proxy))
    log.info('ftp_proxy={}'.format(ftp_proxy))
    log.info('no_proxy={}'.format(no_proxy))
    return http_proxy, https_proxy, ftp_proxy, no_proxy


def check_network(client_interface, controller_interface):
    """Verify the interfaces are usable and return the FORWARD IN rule.

    The environment is also checked that it defines proxy information needed
    to work on a restricted network.

    :raises UndefinedProxyError: when the current env or /etc/environment does
        not define proxy info.
    :raises ValueError: when the interfaces are not present or the FORWARD IN
        rule cannot be identified.
    :return: the FORWARD IN rule that must be deleted to test, then restored.
    """
    if subprocess.call(['ifconfig', client_interface]) != 0:
        message = 'client_interface {} not found'.format(client_interface)
        log.error(message)
        raise ValueError(message)
    if subprocess.call(['ifconfig', controller_interface]) != 0:
        message = 'controller_interface {} not found'.format(
            controller_interface)
        log.error(message)
        raise ValueError(message)
    check_environment()
    # We need to match a single rule from iptables:
    # sudo iptables -S lxdbr0
    # -A FORWARD -i lxdbr0 -m comment --comment "managed by lxd" -j ACCEPT
    rules = subprocess.check_output(['sudo', 'iptables', '-S', 'FORWARD'])
    forward_pattern = re.compile(
        '(-A FORWARD -i {}.*-j ACCEPT)'.format(controller_interface))
    forward_rule = None
    for rule in rules.splitlines():
        match = forward_pattern.search(rule)
        if match and forward_rule is None:
            forward_rule = match.group(1)
        elif match and forward_rule is not None:
            # There is more than one match. We did not match a unique to
            # delete and restore.
            forward_rule = None
            break
    if forward_rule is None:
        # Either the rule was not matched or it was matched more than once.
        raise ValueError(
            'Cannot identify the unique iptables FOWARD IN rule for {}'.format(
                controller_interface))
    return forward_rule


def backup_iptables():
    """Backup iptables so that it can be restored later.

    The backup is to /etc/iptables.before-assess-proxy.

    :raises: CalledProcessError when iptables could not be backed up.
    """
    log.info('Backing up iptables to {}'.format(IPTABLES_BACKUP))
    subprocess.check_call([IPTABLES_BACKUP_BASH], shell=True)


def setup_common_firewall():
    """Setup rules for basic proxy testing.

    These rules ensure ssh in and proxy, dns, dhcp, and ntp are permitted.
    These rules are safe to keep, but unnecessary on open networks.

    :raises: CalledProcessError when ufw cannot add rules.
    """
    log.info('Setting common firewall rules.')
    log.info('These are safe permissive rules.')
    subprocess.check_call([UFW_PROXY_COMMON_BASH], shell=True)


def setup_client_firewall(client_interface):
    """Setup rules for Juju client proxy testing.

    These rules block the localhost's interface to the internet. Call
    setup_common_firewall() first to ensure the host has basic egress.

    :param client-interface: the interface used by the client to access
        the internet. It will be blocked.
    :raises: CalledProcessError when ufw cannot add rules.
    """
    log.info('Setting client firewall rules.')
    log.info(
        'These rules restrict the localhost on {}.'.format(client_interface))
    script = UFW_PROXY_CLIENT_BASH.format(
        interface=client_interface)
    subprocess.check_call([script], shell=True)


def setup_controller_firewall(controller_interface, forward_rule):
    """Setup rules for Juju controller proxy testing.

    These rules block the network interface the controller and its models use.
    Call setup_common_firewall() first to ensure the host has basic egress.

    :param controller-interface: the interface used by the controller to access
        the internet. It will be blocked
    :param forward_rule: the iptables FORWARD IN rule that must be deleted to
         setup then test, then restored later
    :raises: CalledProcessError when ufw or iptables cannot add rules.
    """
    log.info('Setting controller firewall rules.')
    log.info(
        'These rules restrict the controller on {}.'.format(
            controller_interface))
    original_forward_rule = forward_rule.replace('-A FORWARD', 'FORWARD')
    script = UFW_PROXY_CONTROLLER_BASH.format(
        interface=controller_interface,
        original_forward_rule=original_forward_rule)
    subprocess.check_call([script], shell=True)


def set_firewall(scenario,
                 client_interface, controller_interface, forward_rule):
    """Setup the firewall to match the scenario.

    Calling this will create a backup of iptables, then update both ufw and
    iptables to setup the scenario under test.

    :param scenario: the scenario to setup: both-proxied, client-proxied, or
        controller-proxied.
    :param client-interface: the interface used by the client to access
        the internet.
    :param controller-interface: the interface used by the controller to access
        the internet.
    :param forward_rule: the iptables FORWARD IN rule that must be deleted to
         setup then test, then restored later.
    """
    backup_iptables()
    log.info('\nIn case of disaster, the firewall can be restored by running:')
    log.info('sudo iptables-restore {}'.format(IPTABLES_BACKUP))
    log.info('sudo ufw reset\n')
    setup_common_firewall()
    if scenario in (SCENARIO_BOTH, SCENARIO_CLIENT):
        setup_client_firewall(client_interface)
    if scenario in (SCENARIO_BOTH, SCENARIO_CONTROLLER):
        setup_controller_firewall(controller_interface, forward_rule)


def reset_firewall():
    """Reset the firewall and disable it.

    The firewall's rules are reset, then it is disabled. The ufw reset command
    implicitly disables, but disable is explicitly called to ensure ufw
    is not running. iptables-restore it called with the .before-assess-proxy
    backup.
    """
    errors = []
    for command in UFW_RESET_COMMANDS:
        try:
            subprocess.check_call(command)
            log.info('{} exited successfully'.format(command))
        except subprocess.CalledProcessError as e:
            errors.append(e)
            log.error('{} exited with {}'.format(e.cmd, e.returncode))
            log.error('This host may be in a dirty state.')
    return errors


def assess_proxy(client, scenario):
    client.deploy('cs:bionic/ubuntu')
    client.wait_for_started()
    client.wait_for_workloads()
    log.info("SUCCESS")


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Assess Juju under various proxy network conditions.")
    add_basic_testing_arguments(parser)
    parser.add_argument(
        'scenario', choices=[
            SCENARIO_BOTH, SCENARIO_CLIENT, SCENARIO_CONTROLLER],
        help="The proxy scenario to run.")
    parser.add_argument(
        '--client-interface', default='eth0',
        help="The interface used by the client to access the internet.")
    parser.add_argument(
        '--controller-interface', default='lxdbr0',
        help="The interface used by the controller to access the internet.")
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    log.info("Checking the setup of the network and firewall")
    forward_rule = check_network(
        args.client_interface, args.controller_interface)
    try:
        log.info("Setting firewall")
        set_firewall(
            args.scenario, args.client_interface, args.controller_interface,
            forward_rule)
        log.info("Starting test")
        bs_manager = BootstrapManager.from_args(args)
        log.info("Starting bootstrap")
        with bs_manager.booted_context(args.upload_tools):
            log.info("PASS bootstrap")
            assess_proxy(bs_manager.client, args.scenario)
            log.info("Finished test")
    finally:
        # Always reopen the network, regardless of what happened.
        # Do not lockout the host.
        log.info("Resetting firewall")
        reset_firewall()
    return 0


if __name__ == '__main__':
    sys.exit(main())
