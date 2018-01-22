#!/usr/bin/env python
"""Assess network spaces for supported providers (currently only EC2)"""

import argparse
import logging
import sys
import json
import yaml
import subprocess
import re
import os
import ipaddress
import boto3

from jujupy import (
    client_from_config,
    client_for_existing
    )
from jujupy.exceptions import (
    ProvisioningError
    )
from deploy_stack import (
    BootstrapManager
    )
from utility import (
    add_basic_testing_arguments,
    generate_default_clean_dir,
    configure_logging,
    )

__metaclass__ = type

log = logging.getLogger("assess_network_spaces")


class AssessNetworkSpaces:

    def __init__(self, args):
        if args.logs:
            self.log_dir = args.logs
        else:
            self.log_dir = generate_default_clean_dir(
                            args.temp_env_name)

    def assess_network_spaces(self, client, target_model=None, series=None):
        """Assesses network spaces

        :param client: The juju client in use
        :param target_model: Optional existing model to test under
        :param series: Ubuntu series to deploy
        """
        self.setup_testing_environment(client, target_model, series)
        log.info('Starting network tests.')
        failures, failmsg = self.testing_iterations(client)
        if failures:
            raise TestFailure('Test failures:' + failmsg)
        log.info('SUCESS')
        return


    def testing_iterations(self, client):
        """Verify that spaces are set up proper and functioning

        :param client: Juju client object with machines and spaces
        """
        failures = 0
        failmsg = ''
        alltests = [
            self.verify_machine_spaces,
            self.verify_spaces_connectivity,
            self.internet_connection,
            # Do this one last so the failed container doesn't
            # interfere with the other tests.
            self.assert_add_container_with_wrong_space_errs,
        ]

        for test in alltests:
            results = test(client)
            if results['pass']:
                log.info('PASSED: ' + results['message'] + '\n')
            else:
                failures += 1
                failmsg += '\n' + results['message']
                log.info('FAILED: ' + results['message'] + '\n')

        log.info('Tests complete.')
        return(failures, failmsg)


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
        # add machines for spaces testing
        # (should we do this if we are on an existing model?)
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


    def verify_machine_spaces(self, client):
        """Check all the machines to verify they are in the expected spaces
        We should have 4 machines in 3 spaces
        0 and 1 in space1
        2 in space2
        3 in space3

        :param client: Juju client object with machines and spaces
        :returns pass: true/false upon pass/fail
        :returns message: failure message
        """
        log.info('Assessing machines are in the correct spaces.')
        spaces = non_infan_subnets(
            yaml.safe_load(
                client.get_juju_output(
                    'list-spaces', '--format=yaml')))
        machines = yaml.safe_load(
            client.get_juju_output(
                'list-machines', '--format=yaml'))['machines']

        results = {
            'pass': True,
            'message': ''
        }
        for machine in machines.keys():
            if machine == '0':
                expected_space = 'space1'
            else:
                expected_space = 'space{}'.format(machine)
            eth0 = machines[machine]['network-interfaces']['eth0']
            subnet = spaces['spaces'][expected_space].keys()[0]
            for ip in eth0['ip-addresses']:
                if ip_in_cidr(ip, subnet):
                    results['message'] += '\nMachine {machine} was ' \
                        'in the correct space ({space})'.format(
                            machine=machine,
                            space=expected_space)
                    break
                else:
                    results['pass'] = False
                    failmessage = '\nMachine {machine} ip ' \
                        'of eth0 ({ip}) is NOT in {space}({subnet})'.format(
                                machine=machine,
                                ip=ip,
                                space=expected_space,
                                subnet=subnet)
                    results['message'] += failmessage
                    log.error(failmessage)

        return results


    def verify_spaces_connectivity(self, client):
        """Check to make sure machines in the same space can ping
        and that machines in different spaces cannot.
        Machines 0 and 1 are in space1. Ping should succeed.
        Machines 2 and 3 are in space2 and space3. Ping should fail.
        (The second case is not yet implemented in juju spaces.)

        :param client: Juju client object with machines and spaces
        :returns: dict of ping results
        """
        log.info('Assessing interconnectivity between machines.')
        machines = yaml.safe_load(
            client.get_juju_output(
                'list-machines', '--format=yaml'))['machines']
        results = {
            'pass': True,
            'message': ''
        }
        # try 0 to 1
        failmessage = 'Machine 0 should be able to ping machine 1'
        results['message'] +='\n' + failmessage + '\n'
        if machine_can_ping_ip(client, '0',
            machines['1']['network-interfaces']['eth0']['ip-addresses'][0]):
            results['message'] += 'Ping successful: Pass'
        else:
            results['pass'] = False
            log.error(failmessage)
            results['message'] += 'Ping unsuccessful: Fail'
        """Restrictions and access control between spaces is not yet enforced
        # try 2 to 3
        failmessage = 'Machine 2 should not be able to ping machine 3'
        results['message'] +='\n' + failmessage + '\n'
        if machine_can_ping_ip(client, '2',
            machines['3']['network-interfaces']['eth0']['ip-addresses'][0]):
            results['pass'] = False
            log.error(failmessage)
            results['message'] += 'Ping successful: Fail'
        else:
            results['message'] += 'Ping unsuccessful: Pass'
        """
        return results



    def assert_add_container_with_wrong_space_errs(self, client):
        """If we attempt to add a container with a space constraint to a
        machine that already has a space, if the spaces don't match, it
        will fail.

        :param client: Juju client object with machines and spaces
        :returns pass: true/false upon pass/fail
        :returns message: failure message
        """
        log.info('Assessing adding container with wrong space fails.')
        results = {
            'pass': True,
            'message': '\nAdding container with wrong space fails: '
        }
        # add container on machine 2 with space1
        try:
            client.juju(
                'add-machine', ('lxd:2', '--constraints', 'spaces=space1'))
            client.wait_for_started()
            machine = client.show_machine('2')['machines'][0]
            container = machine['containers']['2/lxd/0']
            if container['juju-status']['current'] == 'started':
                log.error('Encountered no conflit when launching a ' \
                    'container on a machine with different spaces ' \
                    'constraint.')
                results['pass'] = False
            results['message'] += str(results['pass'])
        except ProvisioningError:
            log.info('Container correctly failed to provision.')
            results['message'] += 'True'
        finally:
            # clean up container
            try:
                client.wait_for(client.remove_machine('2/lxd/0', force=True))
            except:
                pass
        return(results)


    def internet_connection(self, client):
        """Test that targets can ping their default route.

        :param client: Juju client
        :return: Dict of results by machine
        """
        log.info('Assessing internet connection.')
        results = {
            'pass': True,
            'message': ''
        }
        units = client.get_status().iter_machines(containers=True)
        for unit in units:
            log.info("Assessing internet connection for "
                     "machine: {}".format(unit[0]))
            try:
                routes = client.run(['ip route show'], machines=[unit[0]])
            except subprocess.CalledProcessError:
                failmessage = 'Could not connect to address for unit: ' \
                      '{0}, unable to find default route.'.format(unit[0])
                log.error(failmessage)
                results['pass'] = False
                results['message'] += '\n' + failmessage
                continue
            default_route = re.search(r'(default via )+([\d\.]+)\s+',
                                      json.dumps(routes[0]))
            if default_route:
                results[unit[0]] = True
                results['message'] \
                    += '\nMachine {} has default route.'.format(unit[0])
            else:
                failmessage = "Default route not found for {}".format(unit[0])
                log.error(failmessage)
                results['pass'] = False
                results['message'] += '\n' + failmessage
                continue
        return results


    def deploy_spaces_machines(self, client, series=None):
        """Add machines to test spaces.
        First two machines in the same space, the rest in subsequent spaces.

        :param client: Juju client object with bootstrapped controller
        :param series: Ubuntu series to deploy
        """
        log.info("Adding 4 machines")
        for x in range(0, 4):
            space = x
            if x == 0:
                space = 1
            client.juju(
                'add-machine', (
                    '--series={}'.format(series),
                    '--constraints', 'spaces=space{}'.format(space)))
        client.wait_for_started()


    def cleanup(self, client):
        log.info('Cleaning up launched machines.')
        for x in range(0, 4):
            client.remove_machine(x, force=True)



class SubnetsNotReady(Exception):
    def __init__(self, message):
        super(SubnetsNotReady, self).__init__(message)
        self.message = message


class TestFailure(Exception):
    def __init__(self, message):
        super(TestFailure, self).__init__(message)
        self.message = message


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
    """Launch the test and perform some cleanup."""
    test = AssessNetworkSpaces(args)
    try:
        test.assess_network_spaces(client, args.model, args.series)
    finally:
        if args.model:
            test.cleanup(client)
            log.info('Cleanup complete.')

def vpc_bootstrap(args):
    """Set up the VPC environment before attaching test client

    :param args: args from parse_args
    """
    supported_providers = [ 'ec2' ];
    client = client_from_config(args.env,args.juju_bin)
    current_provider = client.env.provider

    if current_provider not in supported_providers:
        log.info('Skipping tests.\nCurrent provider ({0}) not in supported '
                 'providers ({1}).'.format(
                    current_provider,
                    ', '.join(supported_providers)
                 ))
        return(False)

    if current_provider == 'ec2':
        log.info('Setting up VPC in AWS region {}'.format(
            client.env.get_region()))
        creds = client.env.get_cloud_credentials()
        ec2 = boto3.resource('ec2',
            region_name=client.env.get_region(),
            aws_access_key_id=creds['access-key'],
            aws_secret_access_key=creds['secret-key'])
        # set up vpc
        vpc = ec2.create_vpc(CidrBlock='10.0.0.0/16')
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
        routetable=None
        for rt in vpc.route_tables.all():
            for attrib in rt.associations_attribute:
                if attrib['Main'] == True:
                    routetable=rt
                    break
        # set default route
        route = routetable.create_route(
            DestinationCidrBlock='0.0.0.0/0',
            GatewayId=gateway.id)

        return(vpc.id)


def vpc_cleanup(client, vpcid):
    """Remove VPC from AWS

    :param client: juju client
    :param vpcid: id of vpc to delete
    """
    if not vpcid:
        return
    if client.env.provider != 'ec2':
        return
    log.info('Removing VPC ({vpcid}) from AWS region {region}'.format(
        region=client.env.get_region(),
        vpcid=vpcid))
    creds = client.env.get_cloud_credentials()
    ec2 = boto3.resource('ec2',
        region_name=client.env.get_region(),
        aws_access_key_id=creds['access-key'],
        aws_secret_access_key=creds['secret-key'])
    ec2client = ec2.meta.client
    vpc = ec2.Vpc(vpcid)
    # detach and delete all gateways
    for gw in vpc.internet_gateways.all():
        vpc.detach_internet_gateway(InternetGatewayId=gw.id)
        gw.delete()
    # delete all route table associations
    for rt in vpc.route_tables.all():
        for rta in rt.associations:
            if not rta.main:
                rta.delete()
        main = False
        for attrib in rt.associations_attribute:
            if attrib['Main'] == True:
                    main = True
        if not main:
            rt.delete()
    # delete any instances
    for subnet in vpc.subnets.all():
        for instance in subnet.instances.all():
            instance.terminate()
    # delete our endpoints
    for ep in ec2client.describe_vpc_endpoints(Filters=[{
            'Name': 'vpc-id',
            'Values': [ vpcid ]
        }])['VpcEndpoints']:
        ec2client.delete_vpc_endpoints(VpcEndpointIds=[ep['VpcEndpointId']])
    # delete our security groups
    for sg in vpc.security_groups.all():
        if sg.group_name != 'default':
            sg.delete()
    # delete any vpc peering connections
    for vpcpeer in ec2client.describe_vpc_peering_connections(Filters=[{
            'Name': 'requester-vpc-info.vpc-id',
            'Values': [ vpcid ]
        }] )['VpcPeeringConnections']:
        ec2.VpcPeeringConnection(vpcpeer['VpcPeeringConnectionId']).delete()
    # delete non-default network acls
    for netacl in vpc.network_acls.all():
        if not netacl.is_default:
            netacl.delete()
    # delete network interfaces and subnets
    for subnet in vpc.subnets.all():
        for interface in subnet.network_interfaces.all():
            interface.delete()
        subnet.delete()
    # finally, delete the vpc
    ec2client.delete_vpc(VpcId=vpcid)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    if args.model:
        # if a model is given, assume we're already bootstrapped
        client = client_for_existing(args.juju_bin, os.environ['JUJU_HOME'])
        start_test(client, args)
    else:
        vpcid = vpc_bootstrap(args)
        if not vpcid:
            return 0
        bs_manager = BootstrapManager.from_args(args)
        bs_manager.client.env.update_config({'vpc-id': vpcid})
        try:
            with bs_manager.booted_context(args.upload_tools):
                start_test(bs_manager.client, args)
        finally:
            vpc_cleanup(bs_manager.client, vpcid)
    return 0


if __name__ == '__main__':
    sys.exit(main())
