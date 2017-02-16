#!/usr/bin/env python
"""Assess network health for a given deployment or bundle"""
from __future__ import print_function

import argparse
import logging
import sys
import json
import yaml
import ast
import subprocess
import pdb

from jujupy import client_from_config
from deploy_stack import (
    BootstrapManager,
    )
from jujucharm import (
    local_charm_path,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    )


__metaclass__ = type

log = logging.getLogger("assess_network_health")


class ConnectionError(Exception):
    """Connection failed in some way"""


def assess_network_health(client, bundle=None, target_model=None):
    """Assesses network health for a given deployment or bundle
    :param client: The juju client in use
    :param bundle: Optional bundle to test on
    :param model: Optional existing model to test under
    """
    setup_testing_environment(client, bundle, target_model)
    log.info("Starting network tests")
    agnostic_result = ensure_juju_agnostic_visibility(client)
    log.info('Agnostic result:\n {}'.format(json.dumps(agnostic_result,
                                                       indent=4,
                                                       sort_keys=True)))
    visibility_result = neighbor_visibility(client)
    log.info('Visibility result:\n {}'.format(json.dumps(visibility_result,
                                                         indent=4,
                                                         sort_keys=True)))
    exposed_result = ensure_exposed(client)
    e = 'No exposed units'
    log.info('Exposure result:\n {}'.format(json.dumps(exposed_result,
                                                       indent=4,
                                                       sort_keys=True)) or e)
    parse_final_results(visibility_result, exposed_result)


def setup_testing_environment(client, bundle, target_model):
    """Sets up the testing environment given an option bundle and/or model
    :param client: The juju client in use
    :param bundle: Optional bundle to test on
    :param model: Optional existing model to test under
    """
    log.info("Setting up test environment")
    if target_model:
        connect_to_existing_model(client, target_model)
    if bundle:
        setup_bundle_deployment(client, bundle)
    elif bundle is None and target_model is None:
        setup_dummy_deployment(client)

    charm_path = local_charm_path(charm='network-health', series='trusty',
                                  juju_ver=client.version)
    client.deploy(charm_path)
    client.wait_for_started()
    client.wait_for_workloads()
    services = get_juju_status(client)['applications'].keys()
    services.remove('network-health')
    log.info('Known applications: {}'.format(services))
    for service in services:
        try:
            client.juju('add-relation', (service, 'network-health'))
        except:
            log.info('Could not relate {} & network-health'.format(service))

    client.wait_for_workloads()
    for service in services:
        client.wait_for_subordinate_units(service, 'network-health')


def connect_to_existing_model(client, target_model):
    """Connects to an existing Juju model

    """
    log.info("Connecting to existing model: {}".format(target_model))
    if client.show_model().keys()[0] is not target_model:
        client.switch(target_model)


def setup_dummy_deployment(client):
    log.info("Deploying dummy charm for basic testing")
    dummy_path = local_charm_path(charm='ubuntu', series='trusty',
                                  juju_ver=client.version)
    client.deploy(dummy_path, num=2)
    client.juju('expose', ('ubuntu',))


def setup_bundle_deployment(client, bundle):
    log.info("Deploying bundle specified at {}".format(bundle))
    client.deploy_bundle(bundle)


def get_juju_status(client):
    return client.get_status().status


def ensure_juju_agnostic_visibility(client):
    """Determine if known juju machines are visible
    :param machine: List of machine IPs to test
    :return: Connection attempt results
    """
    machines = get_juju_status(client)['machines']
    pdb.set_trace()
    result = {}
    for machine, info in machines.items():
        result[machine] = {}
        for ip in info['ip-addresses']:
            try:
                output = subprocess.check_output("ping -c 1 " + ip, shell=True)
            except Exception, e:
                result[machine][ip] = False
            result[machine][ip] = True
    return result


def neighbor_visibility(client):
    """Check if each application's units are visible, including our own.
    :param client: The juju client in use
    :param apps: Dict of juju applications
    :param targets: Dict of units & public-addresses by application
    """
    apps = get_juju_status(client)['applications']
    targets = parse_targets(apps)
    result = {}
    nh_units = []
    for service in apps.values():
        for unit in service.get('units', {}).values():
            nh_units.extend(unit.get('subordinates').keys())
    for nh_unit in nh_units:
        service_results = {}
        for service, units in targets.items():
            res = ping_units(client, nh_unit, units)
            service_results[service] = ast.literal_eval(res)
        result[nh_unit] = service_results
    return result


def ensure_exposed(client):
    """Ensure exposed services are visible from the outside
    :param client: The juju client in use
    :param targets: Dict of units & public-addresses by application
    :param exposed: List of exposed services
    :return:
    """
    log.info('Starting test of exposed units')
    apps = get_juju_status(client)['applications']
    targets = parse_targets(apps)
    exposed = [app for app, e in apps.items() if e.get('exposed') is True]
    if len(exposed) is 0:
        log.info('No exposed units, aboring test.')
        return None
    new_client = setup_expose_test(client)
    service_results = {}
    for service, units in targets.items():
        service_results[service] = ping_units(new_client, 'network-health/0',
                                              units)
    return parse_expose_results(service_results, exposed)


def setup_expose_test(client):
    new_client = client.add_model('exposetest')
    dummy_path = local_charm_path(charm='ubuntu', series='trusty',
                                  juju_ver=client.version)
    new_client.deploy(dummy_path)
    charm_path = local_charm_path(charm='network-health', series='trusty',
                                  juju_ver=client.version)
    new_client.deploy(charm_path)
    new_client.wait_for_started()
    new_client.wait_for_workloads()
    new_client.juju('add-relation', ('ubuntu', 'network-health'))
    new_client.wait_for_subordinate_units('ubuntu', 'network-health')
    return new_client


def parse_expose_results(service_results, exposed):
    result = {'fail': (),
              'pass': ()}
    for service, results in service_results.items():
        # If we could connect but shouldn't, fail
        if 'True' in results and service not in exposed:
            result['fail'] = result['fail'] + (service,)
        # If we could connect but should, pass
        elif 'True' in results and service in exposed:
            result['pass'] = result['pass'] + (service,)
        # If we couldn't connect and shouldn't, pass
        elif 'False' in results and service not in exposed:
            result['pass'] = result['pass'] + (service,)
        else:
            result['fail'] = result['fail'] + (service,)
    return result


def parse_final_results(visibility, exposed=None):
    """Parses test results and raises an error if any failed
    :param visibility: Visibility test result
    :param exposed: Exposure test result
    """
    error_string = ''
    for nh_source, service_result in visibility.items():
            for service, unit_res in service_result.items():
                if False in unit_res.values():
                    failed = [u for u, r in unit_res.items() if r is False]
                    error = 'NH-Unit {0} failed to contact ' \
                            'unit(s): {1}\n'.format(nh_source, failed)
                    error_string += error

    if exposed and exposed['fail'] is not ():
        error = 'Service(s) {} failed expose test\n'.format(exposed['fail'])
        error_string += error

    if error_string is not '':
        raise ConnectionError(error_string)


def ping_units(client, source, units):
    """Calls out to our subordinate network-health charm to ping targets
    :param client: The juju client to address
    :param source: The specific network-health unit to send from
    :param units: The units to ping
    """
    units = to_json(units)
    args = "targets='{}'".format(units)
    retval = client.action_do(source, 'ping', args)
    result = client.action_fetch(retval)
    result = yaml.safe_load(result)['results']['results']
    return result


def to_json(units):
    """Returns a formatted json string to be passed through juju run-action
    :param units: Dict of units
    :return: A "JSON-like" string that can be passed to Juju without it puking
    """
    json_string = json.dumps(units, separators=(',', '='))
    # Replace curly brackets so juju doesn't think it's YAML or JSON and puke
    json_string = json_string.replace('{', '(')
    json_string = json_string.replace('}', ')')
    return json_string


def parse_targets(apps):
    """Returns targets based on supplied juju status information.
    :param apps: Dict of applications via 'juju status --format yaml'
    """
    targets = {}
    for app, units in apps.items():
        target_units = {}
        if 'units' in units:
            for unit_id, info in units.get('units').items():
                target_units[unit_id] = info['public-address']
            targets[app] = target_units
    return targets


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test Network Health")
    add_basic_testing_arguments(parser)
    parser.add_argument('--bundle', help='Bundle to test network against')
    parser.add_argument('--model', help='Existing Juju model to test under')
    parser.set_defaults(series='trusty')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    if args.model is None:
        bs_manager = BootstrapManager.from_args(args)
        with bs_manager.booted_context(args.upload_tools):
            assess_network_health(bs_manager.client, bundle=args.bundle)
    else:
        client = client_from_config(args.env, args.juju_bin)
        assess_network_health(client, args.bundle, args.model)
    return 0


if __name__ == '__main__':
    sys.exit(main())
