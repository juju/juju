#!/usr/bin/env python
"""Assess network health for a given deployment or bundle"""
from __future__ import print_function

import argparse
import logging
import sys
import json
import yaml
import pdb

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


def assess_network_health(client, bundle=None):
    """Assesses network health for a given deployment or bundle
    :param client: The juju client in use
    :param bundle: Optional bundle to test on
    """
    if bundle:
        client.deploy_bundle(bundle)
    # Else deploy two dummy charms to test on
    else:
        dummy_path = local_charm_path(charm='ubuntu', series='trusty',
                                      juju_ver=client.version)
        client.deploy(dummy_path, num=2)
        client.juju('expose', ('ubuntu',))
    charm_path = local_charm_path(charm='network-health', series='trusty',
                                  juju_ver=client.version)
    client.deploy(charm_path)
    client.wait_for_started()
    client.wait_for_workloads()
    services = client.get_status().status['applications'].keys()
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
    log.info("Starting network tests")
    apps = client.get_status().status['applications']
    targets = parse_targets(apps)
    log.info(neighbor_visibility(client, apps, targets))
    # Grab exposed charms
    exposed = [app for app, e in apps.items() if e.get('exposed') is True]
    # If we have exposed charms, test their exposure
    if len(exposed) > 0:
        log.info(ensure_exposed(client, targets, exposed))


def neighbor_visibility(client, apps, targets):
    """Check if each application's units are visible, including our own.
    :param client: The juju client in use
    :param apps: Dict of juju applications
    :param targets: Dict of units & public-addresses by application
    """
    results = {}
    nh_units = []
    for service in apps.values():
        try:
            for unit in service.get('units').values():
                nh_units.extend(unit.get('subordinates').keys())
        except:
            continue
    for nh_unit in nh_units:
        service_results = {}
        for service, units in targets.items():
            service_results[service] = ping_units(client, nh_unit, units)
        results[nh_unit] = service_results
    return results


def ensure_exposed(client, targets, exposed):
    """Ensure exposed services are visible from the outside
    :param client: The juju client in use
    :param targets: Dict of units & public-addresses by application
    :param exposed: List of exposed services
    :return:
    """
    # Spin up new client and deploy under it
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
    service_results = {}
    for service, units in targets.items():
        service_results[service] = ping_units(new_client, 'network-health/0',
                                              units)
    # Check revtal against exposed, return passes & failures
    pdb.set_trace()
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
    """
    json_string = json.dumps(units, separators=(',', '='))
    # Replace curly brackets so juju doesn't puke
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
    parser.set_defaults(series='trusty')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        bundle = args.bundle
        assess_network_health(bs_manager.client, bundle)
    return 0


if __name__ == '__main__':
    sys.exit(main())
