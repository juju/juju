#!/usr/bin/env python
"""Assess network spaces for supported providers (currently only EC2)"""

import argparse
import logging
import sys
import json
import yaml
import subprocess
import re
import time
import os
from collections import defaultdict
import ipaddress

from jujupy import (
    client_for_existing
    )
from jujupy.exceptions import (
    ProvisioningError
    )
from jujupy.wait_condition import (
    WaitApplicationNotPresent
    )
from deploy_stack import (
    BootstrapManager
    )
from utility import (
    add_basic_testing_arguments,
    generate_default_clean_dir,
    configure_logging,
    wait_for_port
    )
from substrate import (
    maas_account_from_boot_config,
    )

__metaclass__ = type

log = logging.getLogger("assess_network_spaces")

NO_EXPOSED_UNITS = 'No exposed units'

PORT = 8039


class AssessNetworkSpaces:

    def __init__(self, args):
        if args.logs:
            self.log_dir = args.logs
        else:
            self.log_dir = generate_default_clean_dir(
                            args.temp_env_name)
        self.expose_client = None
        self.existing_series = set([])
        self.expose_test_charms = set([])
        self.supported_spaces_providers = [ 'ec2' ];

    def assess_network_spaces(self, client, target_model=None, series=None):
        """Assesses network spaces

        :param client: The juju client in use
        :param target_model: Optional existing model to test under
        :param series: Ubuntu series to deploy
        """
        self.setup_testing_environment(client, target_model, series)
        log.info('Starting network tests.')
        results_spaces = self.testing_iterations(client)
        error_string = ['Test failures:']
        if results_spaces:
            error_string.extend(results_spaces)
            raise Exception('\n'.join(error_string))
        log.info('SUCESS')
        return


    def testing_iterations(self, client):
        """Verify that spaces are set up proper and functioning
        Currently this only supports EC2.

        :param client: Juju client object with machines and spaces
        """
        if client.env.provider not in self.supported_spaces_providers:
            return

        spaces = non_infan_subnets(
            yaml.safe_load(
                client.get_juju_output(
                    'list-spaces', '--format=yaml')))
        log.info(
            'SPACES:\n {}'.format(
                json.dumps(spaces, indent=4, sort_keys=True)))
        machines = yaml.safe_load(client.get_juju_output('list-machines',
            '--format=yaml'))['machines']
        machine_test = self.verify_machine_spaces(
            client, spaces, machines)
        log.info('Machines in Expected Spaces '
                 'result:\n {0}'.format(json.dumps(machine_test, indent=4,
                                                   sort_keys=True)))

        ping_test = self.verify_spaces_connectivity(client, machines)
        log.info('Ping tests '
                 'result:\n {0}'.format(json.dumps(ping_test, indent=4,
                                                   sort_keys=True)))

        fail_test = self.add_container_with_wrong_space_errs(client)
        log.info('Ensure failure to start container with wrong '
                 'space:\n {0}'.format(json.dumps(fail_test, indent=4,
                                                  sort_keys=True)))

        log.info('Tests complete.')
        return self.parse_spaces_results(machine_test, ping_test, fail_test)


    def setup_testing_environment(self, client, target_model,
                                  series=None):
        """Sets up the testing environment given an option model.

        :param client: The juju client in use
        :param model: Optional existing model to test under
        """
        log.info("Setting up test environment.")
        if target_model:
            self.connect_to_existing_model(client, target_model)
        else:
            self.assign_spaces(client)
            self.setup_dummy_deployment(client, series)
        apps = client.get_status().get_applications()
        for _, info in apps.items():
            self.existing_series.add(info['series'])
        for series in self.existing_series:
            try:
                client.deploy('~juju-qa/network-health', series=series,
                              alias='network-health-{}'.format(series))

            except subprocess.CalledProcessError:
                log.info('Could not deploy network-health-{} as it is already'
                         ' present in the juju deployment.'.format(series))
        client.wait_for_started()
        client.wait_for_workloads()
        # finally, add more machines for spaces testing
        self.deploy_spaces_machines(client, series)


    def connect_to_existing_model(self, client, target_model):
        """Connects to an existing Juju model.

        :param client: Juju client object without bootstrapped controller
        :param target_model: Model to connect to for testing
        """
        log.info("Connecting to existing model: {}".format(target_model))
        if client.show_model().keys()[0] is not target_model:
            client.switch(target_model)


    def assign_spaces(self, client):
        """Assigns spaces to subnets
        Currently this only supports EC2.
        Name the spaces sequentially: space1, space2, space3, etc.
        We require at least 3 spaces.

        :param client: Juju client object with controller
        """
        if client.env.provider not in self.supported_spaces_providers:
            log.info('Skipping spaces assignment. Supported providers are: '
                     '{}'.format(' '.join(self.supported_spaces_providers)))
            return

        log.info('Assigning network spaces on {}.'.format(client.env.provider))
        subnets = yaml.safe_load(client.get_juju_output('list-subnets',
            '--format=yaml'))
        if not subnets:
            # TODO We need to set up subnets beforehand so that we never get here
            raise Exception(
                'No subnets defined in {}'.format(client.env.provider))
        subnet_count = 0
        for subnet in non_infan_subnets(subnets)['subnets'].keys():
            subnet_count += 1
            client.juju('add-space', ('space{}'.format(subnet_count), subnet))
        if subnet_count < 3:
            raise Exception('3 subnets required for spaces assignment. {} '
                            'found.'.format(subnet_count))


    def parse_spaces_results(self, machine_test, ping_test, fail_test):
        """Parses test results and return any errors

        :param machine_test: results from machine spaces verification
        :param ping_test: results from connectivity test
        :param fail_test: true if we failed to start container
        """
        log.info('Parsing results from spaces tests.')
        error_string = []
        for machine, res in machine_test.items():
                if not res:
                        expected_space = 'space0'
                        if machine != '0':
                            expected_space = 'space{}'.format(machine)
                        error = ('Machine {0} had incorrect space. '
                                 'expected: {1}'.format(machine, expected_space))
                        error_string.append(error)
        for pinged, res in ping_test.items():
            if not res:
                error = 'Machine {}: Failed.'.format(pinged)
                error_string.append(error)
        for failed, res in fail_test.items():
            if not res:
                error = ('Starting up container on Machine 2 (space2) with '
                         'a constraint for space1 should have failed '
                         'but it did not.')
                error_string.append(error)
        return error_string


    def verify_machine_spaces(self, client, spaces, machines):
        """Check all the machines to verify they are in the expected spaces
        We should have 4 machines in 3 spaces
        0 and 1 in space1
        2 in space2
        3 in space3

        :param client: Juju client object with machines and spaces
        :param spaces: dict of all the defined spaces
        :param machines: dict of all the defiend machines
        :returns: dict of results by machine
        """
        results = {}
        for machine in machines.keys():
            if machine == '0':
                expected_space = 'space1'
            else:
                expected_space = 'space{}'.format(machine)
            eth0 = machines[machine]['network-interfaces']['eth0']
            results[machine] = False
            subnet = spaces['spaces'][expected_space].keys()[0]
            for ip in eth0['ip-addresses']:
                if ip_in_cidr(ip, subnet):
                    results[machine] = True
                    break
                else:
                    log.info('In machine {machine}, {ip} is not in '
                             '{space}({subnet})'.format(
                                machine=machine,
                                ip=ip,
                                space=expected_space,
                                subnet=subnet))
        # TODO instead of returning results, just return error strings
        return results


    def verify_spaces_connectivity(self, client, machines):
        """Check to make sure machines in the same space can ping
        and that machines in different spaces cannot.
        Machines 0 and 1 are in space1. Ping should succeed.
        Machines 2 and 3 are in space2 and space3. Ping should fail.

        :param client: Juju client object with machines and spaces
        :param machines: dict of all the defined machines
        :returns: dict of ping results
        """
        results = {}
        results['0 can ping 1'] = machine_can_ping_ip(client, '0',
            machines['1']['network-interfaces']['eth0']['ip-addresses'][0])
        # Restrictions and access control between spaces is not yet enforced
        #results['2 cannot ping 3'] = not machine_can_ping_ip(client, '2',
        #    machines['3']['network-interfaces']['eth0']['ip-addresses'][0])
        return results



    def add_container_with_wrong_space_errs(self, client):
        """If we attempt to add a container with a space constraint to a
        machine that already has a space, if the spaces don't match, it
        will fail.

        :param client: Juju client object with machines and spaces
        :returns: true if the add fails
        """
        # add container on machine 2 with space1
        try:
            client.juju('add-machine', ('lxd:2', '--constraints', 'spaces=space1'))
            client.wait_for_started()
        except ProvisioningError:
            return { 'failed to add?': True }
        machine = client.show_machine('2')['machines'][0]
        container = machine['containers']['2/lxd/0']
        if container['juju-status']['current'] == 'started':
            return { 'failed to add?': False }
        else:
            return { 'failed to add?': True }


    def setup_dummy_deployment(self, client, series):
        """Sets up a dummy test environment with 2 ubuntu charms.

        :param client: Bootstrapped juju client
        """
        log.info("Deploying dummy charm for basic testing.")
        ## TODO add test here to only add constraint if we're testing spaces
        client.deploy('ubuntu', num=2, series=series,
            constraints='spaces=space1')
        client.juju('expose', ('ubuntu',))
        client.wait_for_started()
        client.wait_for_workloads()

    def deploy_spaces_machines(self, client, series=None):
        """Add a couple of extra machines to test spaces.
        Currently only supported on EC2.

        :param client: Juju client object with bootstrapped controller
        :param series: Ubuntu series to deploy
        """
        if client.env.provider not in self.supported_spaces_providers:
            return

        log.info("Adding network spaces machines")
        client.juju('add-machine', ('--series={}'.format(series),
                                    '--constraints', 'spaces=space2'))
        client.juju('add-machine', ('--series={}'.format(series),
                                    '--constraints', 'spaces=space3'))
        client.wait_for_started()


    def cleanup(self, client):
        log.info('Cleaning up launched machines.')
        # TODO remove machines created for spaces testing
        client.remove_machine('2', force=True)
        client.remove_machine('3', force=True)


    def internet_connection(self, client):
        """Test that targets can ping their default route.

        :param client: Juju client
        :return: Dict of results by machine
        """
        log.info('Assessing internet connection.')
        results = {}
        units = client.get_status().iter_machines(containers=True)
        for unit in units:
            log.info("Assessing internet connection for "
                     "machine: {}".format(unit[0]))
            results[unit[0]] = False
            try:
                routes = client.run(['ip route show'], machines=[unit[0]])
            except subprocess.CalledProcessError:
                log.error('Could not connect to address for unit: {0}, '
                          'unable to find default route.'.format(unit[0]))
                continue
            default_route = re.search(r'(default via )+([\d\.]+)\s+',
                                      json.dumps(routes[0]))
            if default_route:
                results[unit[0]] = True
            else:
                log.error("Default route not found for {}".format(unit[0]))
                continue
        return results


    def curl(self, ip):
        log.info('Attempting to curl unit at {}:{}'.format(ip, PORT))
        try:
            out = subprocess.check_output(
                'curl {}:{} -m 5'.format(ip, PORT), shell=True)
        except subprocess.CalledProcessError as e:
            out = ''
            log.warning('Curl failed for error:\n{}'.format(e))
        log.info('Got: "{}" from unit at {}:{}'.format(out, ip, PORT))
        if 'pass' in out:
            return True
        return False


    def ssh(self, client, machine, cmd):
        """Convenience function: run a juju ssh command and get back the output
        :param client: A Juju client
        :param machine: ID of the machine on which to run a command
        :param cmd: the command to run
        :return: text output of the command
        """
        back_off = 2
        attempts = 4
        for attempt in range(attempts):
            try:
                return client.get_juju_output('ssh', '--proxy', machine,
                                              cmd)
            except subprocess.CalledProcessError as e:
                # If the connection to the host failed, try again in a couple
                # of seconds. This is usually due to heavy load.
                if(attempt < attempts - 1 and
                    re.search('ssh_exchange_identification: '
                              'Connection closed by remote host', e.stderr)):
                    time.sleep(back_off)
                    back_off *= 2
                else:
                    raise


def non_infan_subnets(subnets):
    """Returns all subnets that don't have INFAN in the provider-id
    Subnets with INFAN in the provider-id may be inherited from underlay
    and therefore cannot be assigned to a space.

    :param subnets: A dict of subnets or spaces as returned by
                    juju list-subnets or  juju list-spaces
    """

    """ Example dict output from juju list-subnets:
        "subnets": {
            "172.31.0.0/20": {
                "provider-id": "subnet-38f9d07e",
                "provider-network-id": "vpc-1f40b47a",
                "space": "",
                "status": "in-use",
                "type": "ipv4",
                "zones": [
                    "us-east-1a"
                ]
             }
         }
    """
    newsubnets = {}
    if 'subnets' in subnets:
        newsubnets['subnets'] = {}
        for subnet in subnets['subnets'].keys():
            if 'INFAN' not in subnets['subnets'][subnet]['provider-id']:
                newsubnets['subnets'][subnet] = subnets['subnets'][subnet]

    """ Example dict output from juju list-spaces:
        "spaces": {
            "space1": {
                "172.31.16.0/20": {
                    "provider-id": "subnet-13a6aa67",
                    "status": "in-use",
                    "type": "ipv4",
                    "zones": [
                        "us-east=1d"
                    ]
                }
            }
        }
    """
    if 'spaces' in subnets:
        newsubnets['spaces'] = {}
        for space in subnets['spaces'].keys():
            for subnet in subnets['spaces'][space].keys():
                if ('INFAN' not in
                        subnets['spaces'][space][subnet]['provider-id']):
                    newsubnets['spaces'].setdefault(space, {})
                    newsubnets['spaces'][space][subnet] = \
                        subnets['spaces'][space][subnet]

    return newsubnets


def machine_can_ping_ip(client, machine, ip):
    """SSH to the machine and attempt to ping the given IP.

    :param client: juju client object
    :param machine: machine to connect to
    :param ip: IP address to ping
    :returns: success of ping
    """
    rc, _ = client.juju('ssh',
                ('--proxy', machine, 'ping -c1 -q ' + ip), check=False)
    return rc == 0


def ip_in_cidr(address, cidr):
    """Returns true if the ip address given is within the range defined
    by the cidr subnet.

    :param address: A valid IPv4 address
    :param cidr: A valid subnet in CIDR notation
    """
    return (ipaddress.ip_address(address.decode('utf-8'))
            in ipaddress.ip_network(cidr.decode('utf-8')))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test Network Spaces")
    add_basic_testing_arguments(parser)
    parser.add_argument('--model', help='Existing Juju model to test against')
    parser.set_defaults(series='xenial')
    return parser.parse_args(argv)


def start_test(client, args):
    test = AssessNetworkSpaces(args)
    try:
        test.assess_network_spaces(client, args.model, args.series)
    finally:
        if args.model:
            test.cleanup(client)
            log.info('Cleanup complete.')


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    if args.model:
        client = client_for_existing(args.juju_bin, os.environ['JUJU_HOME'])
        start_test(client, args)
    else:
        bs_manager = BootstrapManager.from_args(args)
        with bs_manager.booted_context(args.upload_tools):
            start_test(bs_manager.client, args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
