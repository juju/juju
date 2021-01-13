#!/usr/bin/env python3
"""Assess network spaces for supported providers (currently only EC2)"""

import argparse
import logging
import sys
import json
import yaml
import subprocess
import re
import ipaddress
import boto3

from deploy_stack import (
    BootstrapManager
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    )
from jujupy.exceptions import (
    ProvisioningError
    )

__metaclass__ = type

log = logging.getLogger("assess_network_spaces")


class AssessNetworkSpaces:

    def assess_network_spaces(self, client, series=None):
        """Assesses network spaces

        :param client: The juju client in use
        :param series: Ubuntu series to deploy
        """
        self.setup_testing_environment(client, series)
        log.info('Starting spaces tests.')
        self.testing_iterations(client)
        # if we get here, tests succeeded
        log.info('SUCCESS')
        return

    def testing_iterations(self, client):
        """Verify that spaces are set up proper and functioning

        :param client: Juju client object with machines and spaces
        """
        alltests = [
            self.assert_machines_in_correct_spaces,
            self.assert_machine_connectivity,
            self.assert_internet_connection,
            # Do this one last so the failed container doesn't
            # interfere with the other tests.
            self.assert_add_container_with_wrong_space_errs,
        ]

        fail_messages = []
        for test in alltests:
            try:
                test(client)
            except TestFailure as e:
                fail_messages.append(str(e))
                log.info('FAILED: ' + str(e) + '\n')

        log.info('Tests complete.')
        if fail_messages:
            raise TestFailure('\n'.join(fail_messages))

    def setup_testing_environment(self, client, series=None):
        """Sets up the testing environment

        :param client: The juju client in use
        """
        log.info("Setting up test environment.")
        self.assign_spaces(client)
        # add machines for spaces testing
        self.deploy_spaces_machines(client, series)

    def assign_spaces(self, client):
        """Assigns spaces to subnets
        Name the spaces sequentially: space1, space2, space3, etc.
        We require at least 3 spaces.

        :param client: Juju client object with controller
        """
        log.info('Assigning network spaces on {}.'.format(client.env.provider))
        subnets = yaml.safe_load(client.get_juju_output('list-subnets',
                                                        '--format=yaml'))
        if not subnets:
            raise SubnetsNotReady(
                'No subnets defined in {}'.format(client.env.provider))
        subnet_count = 0
        for subnet in non_infan_subnets(subnets)['subnets'].keys():
            subnet_count += 1
            client.juju('add-space', ('space{}'.format(subnet_count), subnet))
        if subnet_count < 3:
            raise SubnetsNotReady('3 subnets required for spaces assignment. '
                                  '{} found.'.format(subnet_count))

    def assert_machines_in_correct_spaces(self, client):
        """Check all the machines to verify they are in the expected spaces
        We should have 4 machines in 3 spaces
        0 and 1 in space1
        2 in space2
        3 in space3

        :param client: Juju client object with machines and spaces
        """
        log.info('Assessing machines are in the correct spaces.')
        machines = yaml.safe_load(
            client.get_juju_output(
                'list-machines', '--format=yaml'))['machines']
        for machine in machines.keys():
            log.info('Checking network space for Machine {}'.format(machine))
            if machine == '0':
                expected_space = 'space1'
            else:
                expected_space = 'space{}'.format(machine)
            ip = get_machine_ip_in_space(client, machine, expected_space)
            if not ip:
                raise TestFailure('Machine {machine} has NO IPs in '
                                  '{space}'.format(
                                      machine=machine,
                                      space=expected_space))
        log.info('PASSED')

    def assert_machine_connectivity(self, client):
        """Check to make sure machines in the same space can ping
        and that machines in different spaces cannot.
        Machines 0 and 1 are in space1. Ping should succeed.
        Machines 2 and 3 are in space2 and space3. Ping should succeed.
        We don't currently have access control between spaces.
        In the future, pinging between different spaces may be
        restrictable.

        :param client: Juju client object with machines and spaces
        """
        log.info('Assessing interconnectivity between machines.')
        # try 0 to 1
        log.info('Testing ping from Machine 0 to Machine 1 (same space)')
        ip_to_ping = get_machine_ip_in_space(client, '1', 'space1')
        if not machine_can_ping_ip(client, '0', ip_to_ping):
            raise TestFailure('Ping from 0 to 1 Failed.')
        # try 2 to 3
        log.info('Testing ping from Machine 2 to Machine 3 (diff spaces)')
        ip_to_ping = get_machine_ip_in_space(client, '3', 'space3')
        if not machine_can_ping_ip(client, '2', ip_to_ping):
            raise TestFailure('Ping from 2 to 3 Failed.')
        log.info('PASSED')

    def assert_add_container_with_wrong_space_errs(self, client):
        """If we attempt to add a container with a space constraint to a
        machine that already has a space, if the spaces don't match, it
        will fail.

        :param client: Juju client object with machines and spaces
        """
        log.info('Assessing adding container with wrong space fails.')
        # add container on machine 2 with space1
        try:
            client.juju(
                'add-machine', ('lxd:2', '--constraints', 'spaces=space1'))
            client.wait_for_started()
            machine = client.show_machine('2')['machines'][0]
            container = machine['containers']['2/lxd/0']
            if container['juju-status']['current'] == 'started':
                raise TestFailure(('Encountered no conflict when launching a '
                                   'container on a machine with a different '
                                   'spaces constraint.'))
        except ProvisioningError:
            log.info('Container correctly failed to provision.')
        finally:
            # clean up container
            try:
                # this doesn't seem to wait for removal
                client.wait_for(client.remove_machine('2/lxd/0', force=True))
            except Exception:
                pass
        log.info('PASSED')

    def assert_internet_connection(self, client):
        """Test that targets can ping their default route.

        :param client: Juju client
        """
        log.info('Assessing internet connection.')
        for unit in client.get_status().iter_machines(containers=False):
            log.info("Assessing internet connection for "
                     "machine: {}".format(unit[0]))
            try:
                routes = client.run(['ip route show'], machines=[unit[0]])
            except subprocess.CalledProcessError:
                raise TestFailure(('Could not connect to address for unit: {0}'
                                   ', unable to find default route.').format(
                                       unit[0]))
            default_route = re.search(r'(default via )+([\d\.]+)\s+',
                                      json.dumps(routes[0]))
            if not default_route:
                raise TestFailure('Default route not found for {}'.format(
                    unit[0]))
        log.info('PASSED')

    def deploy_spaces_machines(self, client, series=None):
        """Add machines to test spaces.
        First two machines in the same space, the rest in subsequent spaces.

        :param client: Juju client object with bootstrapped controller
        :param series: Ubuntu series to deploy
        """
        log.info("Adding 4 machines")
        for space in [1, 1, 2, 3]:
            client.juju(
                'add-machine', (
                    '--series={}'.format(series),
                    '--constraints', 'spaces=space{}'.format(space)))
        client.wait_for_started()


class SubnetsNotReady(Exception):
    pass


class TestFailure(Exception):
    pass


def non_infan_subnets(subnets):
    """Returns all subnets that don't have INFAN in the provider-id
    Subnets with INFAN in the provider-id may be inherited from underlay
    and therefore cannot be assigned to a space.

    :param subnets: A dict of subnets or spaces as returned by
                    juju list-subnets or juju list-spaces

    Example dict output from juju list-subnets:
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
    Example list output from juju list-spaces:
        "spaces": {
            [
                "id": "1",
                "name": "one-space",
                "subnets": {
                    "172.31.0.0/20": {
                        "type": "ipv4",
                        "provider-id": "subnet-9b4ed4fc",
                        "status": "in-use",
                        "zones": [
                            "us-east-1c"
                        ]
                    },
                    "252.0.0.0/12": {
                        "type": "ipv4",
                        "provider-id": "subnet-9b4ed4fc-INFAN-172-31-0-0-20",
                        "status": "in-use",
                        "zones": [
                            "us-east-1c"
                        ]
                    }
                }
            ]
        }
    """
    newsubnets = {}
    if 'subnets' in subnets:
        newsubnets['subnets'] = {}
        for subnet, details in subnets['subnets'].iteritems():
            if 'INFAN' not in details['provider-id']:
                newsubnets['subnets'][subnet] = details
    if 'spaces' in subnets:
        newsubnets['spaces'] = {}
        for details in subnets['spaces']:
            space = details['name']
            for subnet, subnet_details in details['subnets'].iteritems():
                if 'INFAN' not in subnet_details['provider-id']:
                    newsubnets['spaces'].setdefault(space, {})
                    newsubnets['spaces'][space][subnet] = subnet_details
    return newsubnets


def get_machine_ip_in_space(client, machine, space):
    """Given a machine id and a space name, will return an IP that
    the machine has in the given space.

    :param client:  juju client object with machines and spaces
    :param machine: string. ID of machine to check.
    :param space:   string. Name of space to look for.
    :return ip:     string. IP address of machine in requested space.
    """
    machines = yaml.safe_load(
        client.get_juju_output(
            'list-machines', '--format=yaml'))['machines']
    spaces = non_infan_subnets(
        yaml.safe_load(
            client.get_juju_output(
                'list-spaces', '--format=yaml')))
    subnet = spaces['spaces'][space].keys()[0]
    for ip in machines[machine]['ip-addresses']:
        if ip_in_cidr(ip, subnet):
            return ip


def machine_can_ping_ip(client, machine, ip):
    """SSH to the machine and attempt to ping the given IP.

    :param client: juju client object
    :param machine: machine to connect to
    :param ip: IP address to ping
    :returns: success of ping
    """
    rc, _ = client.juju('ssh', ('--proxy', machine, 'ping -c1 -q ' + ip),
                        check=False)
    return rc == 0


def ip_in_cidr(address, cidr):
    """Returns true if the ip address given is within the range defined
    by the cidr subnet.

    :param address: A valid IPv4 address (string)
    :param cidr: A valid subnet in CIDR notation (string)
    """
    return (ipaddress.ip_address(address.decode('utf-8'))
            in ipaddress.ip_network(cidr.decode('utf-8')))


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test Network Spaces")
    add_basic_testing_arguments(parser)
    parser.set_defaults(series='bionic')
    return parser.parse_args(argv)


def get_spaces_object(client):
    """Returns the appropriate Spaces object based on the client provider

    :param client: A juju client object
    """
    if client.env.provider == 'ec2':
        return SpacesAWS()
    else:
        log.info('Spaces not supported with current provider '
                 '({}).'.format(client.env.provider))


class Spaces:

    def pre_bootstrap(self, client):
        pass

    def cleanup(self, client):
        pass


class SpacesAWS(Spaces):

    def pre_bootstrap(self, client):
        """AWS specific function for setting up the VPC environment before
        doing the bootstrap

        :param client: juju client object
        """

        if client.env.provider != 'ec2':
            log.info('Skipping tests. Requires AWS EC2.')
            return False

        creds = client.env.get_cloud_credentials()
        ec2 = boto3.resource('ec2',
                             region_name=client.env.get_region(),
                             aws_access_key_id=creds['access-key'],
                             aws_secret_access_key=creds['secret-key'])

        # See if the VPC we want already exists.
        vpc_response = ec2.meta.client.describe_vpcs(Filters=[{
            'Name': 'tag:Name',
            'Values': ['assess-network-spaces']
        }])
        vpcs = vpc_response['Vpcs']
        if vpcs:
            self.vpcid = vpcs[0]['VpcId']
            log.info('Reusing VPC {}'.format(self.vpcid))
            client.env.update_config({'vpc-id': self.vpcid})
            return True

        # Set up a VPC if we did not find one.
        log.info('Setting up VPC in AWS region {}'.format(
            client.env.get_region()))
        vpc = ec2.create_vpc(CidrBlock='10.0.0.0/16')
        vpc.create_tags(Tags=[{'Key': 'Name',
                               'Value': 'assess-network-spaces'}])
        vpc.wait_until_available()

        self.vpcid = vpc.id
        # get the first availability zone
        zones = ec2.meta.client.describe_availability_zones()
        firstzone = zones['AvailabilityZones'][0]['ZoneName']
        # create 3 subnets
        for x in range(0, 3):
            subnet = ec2.create_subnet(
                CidrBlock='10.0.{}.0/24'.format(x),
                AvailabilityZone=firstzone,
                VpcId=vpc.id)
            ec2.meta.client.modify_subnet_attribute(
                MapPublicIpOnLaunch={'Value': True},
                SubnetId=subnet.id)
        # add an internet gateway
        gateway = ec2.create_internet_gateway()
        gateway.attach_to_vpc(VpcId=vpc.id)
        # get the main routing table
        routetable = None
        for rt in vpc.route_tables.all():
            for attrib in rt.associations_attribute:
                if attrib['Main']:
                    routetable = rt
                    break
        # set default route
        routetable.create_route(
            DestinationCidrBlock='0.0.0.0/0',
            GatewayId=gateway.id)
        # finally, update the juju client environment with the vpcid
        client.env.update_config({'vpc-id': vpc.id})
        return True

    def cleanup(self, client):
        """Remove VPC from AWS

        :param client: juju client
        """
        if not self.vpcid:
            return
        if client.env.provider != 'ec2':
            return
        log.info('Removing VPC ({vpcid}) from AWS region {region}'.format(
            region=client.env.get_region(),
            vpcid=self.vpcid))
        creds = client.env.get_cloud_credentials()
        ec2 = boto3.resource('ec2',
                             region_name=client.env.get_region(),
                             aws_access_key_id=creds['access-key'],
                             aws_secret_access_key=creds['secret-key'])
        vpc = ec2.Vpc(self.vpcid)
        # delete any instances
        for subnet in vpc.subnets.all():
            for instance in subnet.instances.all():
                instance.terminate()


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    bs_manager = BootstrapManager.from_args(args)
    # The bs_manager.client env's region doesn't normally get updated
    # until we've bootstrapped. Let's force an early update.
    bs_manager.client.env.set_region(bs_manager.region)
    spaces = get_spaces_object(bs_manager.client)
    if not spaces.pre_bootstrap(bs_manager.client):
        return 0
    try:
        with bs_manager.booted_context(args.upload_tools):
            test = AssessNetworkSpaces()
            test.assess_network_spaces(bs_manager.client, args.series)
    finally:
        spaces.cleanup(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
