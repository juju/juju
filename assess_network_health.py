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
    # If a bundle is supplied, deploy it
    if bundle:
        client.deploy_bundle(bundle)
    # Else deploy two dummy charms to test on
    else:
        dummy_path = local_charm_path(charm='ubuntu', series='trusty',
                                      juju_ver=client.version)
        client.deploy(dummy_path, num=2)
    charm_path = local_charm_path(charm='network-health', series='trusty',
                                  juju_ver=client.version)
    client.deploy(charm_path)
    # Wait for the deployment to finish.
    client.wait_for_started()
    client.wait_for_workloads()
    # Grab services from status
    services = client.get_status().status['applications'].keys()
    services.remove('network-health')
    log.info('Known applications: {}'.format(services))
    for service in services:
        try:
            client.juju('add-relation', (service, 'network-health'))
        except:
            log.info('Could not relate {} & network-health'.format(service))

    # Wait again for network-health to deploy
    client.wait_for_workloads()
    for service in services:
        client.wait_for_subordinate_units(service, 'network-health')
    log.info("Starting network tests")
    # Get full status info from juju
    apps = client.get_status().status['applications']
    # Formulate a list of targets from status info
    targets = parse_targets(apps)
    log.info(neighbor_visibility(client, apps, targets))
    # Expose dummy charm if no bundle is specified
    if bundle is None:
        client.juju('expose', ('ubuntu',))
    # Grab exposed charms
    exposed = [app for app, e in apps.items() if e.get('exposed') is True]
    # If we have exposed charms, test their exposure
    if len(exposed) > 0:
        log.info(ensure_exposed(client, targets, exposed))


def neighbor_visibility(client, apps, targets):
    """Check if each application's units are visible, including our own.
    :param targets: Dict of units & public-addresses by application
    """
    results = {}
    # For each application grab our network-health subordinates
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
    :param targets: Dict of units & public-addresses by application
    :param exposed: List of exposed services
    """
    # Spin up new client and deploy under it
    new_client = client.add_model(client.env)
    new_client.wait_for_started()
    dummy_path = local_charm_path(charm='ubuntu', series='trusty',
                                  juju_ver=client.version)
    new_client.deploy(dummy_path)
    charm_path = local_charm_path(charm='network-health', series='trusty',
                                  juju_ver=client.version)
    new_client.deploy(charm_path)
    new_client.juju('add-relation' ('ubuntu', 'network-health'))
    new_client.wait_for_workloads()

    # For each service, try to ping it from the outside model.
    service_results = {}
    for service, units in targets.items():
        service_results[service] = ping_units(new_client, 'network-health/0',
                                              units)
    # Check revtal against exposed, return passes & failures
    fails = []
    passes = []
    for service, returns in service_results:
        if True in returns and service not in exposed:
            fails.append(service)
        elif True in returns and service in exposed:
            passes.append(service)
    return passes, failures


def ping_units(client, source, units):
    # Change our dictionary into a json string
    units = to_json(units)
    args = "targets='{}'".format(units)
    # Ping the supplied units
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
