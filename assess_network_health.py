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
import re
import time
import os
import socket

from jujupy import (
    client_for_existing,
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


__metaclass__ = type

log = logging.getLogger("assess_network_health")

NO_EXPOSED_UNITS = 'No exposed units'


class AssessNetworkHealth:

    def __init__(self, args):
        if args.logs:
            self.log_dir = args.logs
        else:
            self.log_dir = generate_default_clean_dir(
                            args.temp_env_name)
        self.expose_client = None
        self.existing_series = set([])

    def assess_network_health(self, client, bundle=None, target_model=None,
                              reboot=False, series=None):
        """Assesses network health for a given deployment or bundle.

        :param client: The juju client in use
        :param bundle: Optional bundle to test on
        :param model: Optional existing model to test under
        """
        self.setup_testing_environment(client, bundle, target_model, series)
        log.info('Starting network tests.')
        results_pre = self.testing_iterations(client, series, target_model)
        error_string = ['Initial test failures:']
        if not reboot:
            if results_pre:
                error_string.extend(results_pre)
                raise Exception('\n'.join(error_string))
            log.info('SUCESS')
            return
        log.info('Units completed pre-reboot tests, rebooting machines.')
        self.reboot_machines(client)
        results_post = self.testing_iterations(client, series, target_model,
                                               reboot_msg='Post-reboot ')
        if results_pre or results_post:
            error_string.extend(results_pre or 'No pre-reboot failures.')
            error_string.extend(['Post-reboot test failures:'])
            error_string.extend(results_post or 'No post-reboot failures.')
            raise Exception('\n'.join(error_string))
        log.info('SUCESS')
        return

    def testing_iterations(self, client, series, target_model, reboot_msg=''):
        """Runs through each test given for a given client and series

        :param client: Client
        """
        con_result = self.juju_controller_visibility(client)
        log.info('{0}Controller Visibility '
                 'result:\n {1}'.format(reboot_msg,
                                        json.dumps(con_result, indent=4,
                                                   sort_keys=True)))
        int_result = self.internet_connection(client)
        log.info('{0}Internet Test '
                 'result:\n {1}'.format(reboot_msg,
                                        json.dumps(int_result, indent=4,
                                                   sort_keys=True)))
        vis_result = self.neighbor_visibility(client)
        log.info('{0}Visibility '
                 'result:\n {1}'.format(reboot_msg,
                                        json.dumps(vis_result,
                                                   indent=4,
                                                   sort_keys=True)))
        exp_result = None
        if not target_model:
            exp_result = self.ensure_exposed(client, series)
            log.info('{0}Exposure '
                     'result:\n {1}'.format(reboot_msg,
                                            json.dumps(exp_result,
                                                       indent=4,
                                                       sort_keys=True)) or
                     NO_EXPOSED_UNITS)
        log.info('Tests complete.')
        return self.parse_final_results(con_result, vis_result, int_result,
                                        exp_result)

    def setup_testing_environment(self, client, bundle, target_model,
                                  series=None):
        """Sets up the testing environment given an option bundle and/or model.

        :param client: The juju client in use
        :param bundle: Optional bundle to test on or None
        :param model: Optional existing model to test under
        """
        log.info("Setting up test environment.")
        if target_model:
            self.connect_to_existing_model(client, target_model)
        if bundle:
            self.setup_bundle_deployment(client, bundle)
        elif bundle is None and target_model is None:
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
        apps = client.get_status().get_applications()
        log.info('Known applications: {}'.format(apps.keys()))
        for app, info in apps.items():
            app_series = info['series']
            try:
                client.juju('add-relation',
                            (app, 'network-health-{}'.format(app_series)))
            except subprocess.CalledProcessError as e:
                log.error('Could not relate {0} & network-health due '
                          'to error: {1}'.format(app, e))
        client.wait_for_workloads()
        for app, info in apps.items():
            app_series = info['series']
            client.wait_for_subordinate_units(
                app, 'network-health-{}'.format(app_series))

    def connect_to_existing_model(self, client, target_model):
        """Connects to an existing Juju model.

        :param client: Juju client object without bootstrapped controller
        :param target_model: Model to connect to for testing
        """
        log.info("Connecting to existing model: {}".format(target_model))
        if client.show_model().keys()[0] is not target_model:
            client.switch(target_model)

    def setup_dummy_deployment(self, client, series):
        """Sets up a dummy test environment with 2 ubuntu charms.

        :param client: Bootstrapped juju client
        """
        log.info("Deploying dummy charm for basic testing.")
        client.deploy('ubuntu', num=2, series=series)
        client.juju('expose', ('ubuntu',))
        client.wait_for_started()
        client.wait_for_workloads()

    def setup_bundle_deployment(self, client, bundle):
        """Deploys a test environment with supplied bundle.

        :param bundle: Path to a bundle
        """
        log.info("Deploying bundle specified at {}".format(bundle))
        client.deploy_bundle(bundle)
        client.wait_for_started()
        client.wait_for_workloads()

    def cleanup(self, client):
        log.info('Cleaning up deployed test charms and models.')
        for series in self.existing_series:
            client.remove_service('network-health-{}'.format(series))
        if 'exposetest' in client.get_models().keys():
            client.get_models()['exposetest'].destroy_model()
        log.info('Cleanup complete.')

    def juju_controller_visibility(self, client):
        """Determine if known juju machines are visible from controller.

        :param machine: List of machine IPs to test
        :return: Connection attempt results
        """
        cont_client = client.get_controller_client()
        log.info('Starting controller visibility test')
        machines = client.get_status().iter_machines(containers=True)
        result = {}
        for machine, info in machines:
            result[machine] = {}
            for ip in info['ip-addresses']:
                if self.is_ipv6(ip):
                    cmd = 'ping6'
                else:
                    cmd = 'ping'
                result[machine][ip] = False
                try:
                    self.ssh(cont_client, '0', "{} -c 1 ".format(cmd) + ip)
                except subprocess.CalledProcessError as e:
                    log.error('Error with ping attempt '
                              'to {}: {}'.format(ip, e))
                    continue
                result[machine][ip] = True
        return result

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
                routes = self.ssh(client, unit[0], 'ip route show')
            except subprocess.CalledProcessError as e:
                log.error('Could not connect to address for unit: {0}, '
                          'unable to find default route.'.format(unit[0]))
                continue
            default_route = re.search(r'^default\s+via\s+([\d\.]+)\s+', routes,
                                      re.MULTILINE)
            if default_route:
                rc = client.juju('ssh', ('--proxy', unit[0],
                                         'ping -c1 -q ' +
                                         default_route.group(1)),
                                 check=False)
                if rc != 0:
                    log.error('{} unable to ping default route'.format(unit))
                    continue
            else:
                log.error("Default route not found")
                continue
            results[unit[0]] = True
        return results

    def neighbor_visibility(self, client):
        """Check if each application's units are visible, including our own.

        :param client: The juju client in use
        """
        log.info('Starting neighbor visibility test')
        apps = client.get_status().get_applications()
        targets = self.parse_targets(client.get_status())
        result = {}
        nh_units = []
        for service in apps.values():
            for unit in service.get('units', {}).values():
                nh_subs = [u for u in unit.get('subordinates').keys()
                           if 'network-health' in u]
                nh_units.extend(nh_subs)
        for nh_unit in nh_units:
            service_results = {}
            for service, units in targets.items():
                res = self.ping_units(client, nh_unit, units)
                service_results[service] = ast.literal_eval(res)
            result[nh_unit] = service_results
        return result

    def ensure_exposed(self, client, series):
        """Ensure exposed applications are visible from the outside.

        :param client: The juju client in use
        :return: Exposure test results in dict by pass/fail
        """
        log.info('Starting test of exposed units.')
        apps = client.get_status().get_applications()
        targets = self.parse_targets(client.get_status())
        exposed = [app for app, e in apps.items() if e.get('exposed') is True]
        if len(exposed) is 0:
            log.info('No exposed units, aboring test.')
            return None
        new_client = self.setup_expose_test(client, series)
        service_results = {}
        for service, units in targets.items():
            service_results[service] = self.ping_units(new_client,
                                                       'network-health/0',
                                                       units)
        log.info(service_results)
        return self.parse_expose_results(service_results, exposed)

    def setup_expose_test(self, client, series):
        """Sets up new model to run exposure test.

        :param client: The juju client in use
        :return: New juju client object
        """
        if not self.expose_client:
            new_client = client.add_model('exposetest')
            new_client.deploy('ubuntu', series=series)
            new_client.deploy('~juju-qa/network-health', series=series)
            new_client.wait_for_started()
            new_client.wait_for_workloads()
            new_client.juju('add-relation', ('ubuntu', 'network-health'))
            new_client.wait_for_subordinate_units('ubuntu', 'network-health')
            self.expose_client = new_client

        return self.expose_client

    def parse_expose_results(self, service_results, exposed):
        """Parses expose test results into dict of pass/fail.

        :param service_results: Raw results from expose test
        :return: Parsed results dict
        """
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

    def parse_final_results(self, controller, visibility, internet,
                            exposed):
        """Parses test results and raises an error if any failed.

        :param controller: Controller test result
        :param visibility: Visibility test result
        :param exposed: Exposure test result
        """
        log.info('Parsing final results.')
        error_string = []
        for machine, machine_result in controller.items():
            for ip, res in machine_result.items():
                if res is False:
                    error = ('Failed to contact controller from machine {0} '
                             'at address {1}'.format(machine, ip))
                    error_string.append(error)
        for nh_source, service_result in visibility.items():
                for service, unit_res in service_result.items():
                    if False in unit_res.values():
                        failed = [u for u, r in unit_res.items() if r is False]
                        error = ('NH-Unit {0} failed to contact '
                                 'unit(s): {1}'.format(nh_source, failed))
                        error_string.append(error)
        for unit, res in internet.items():
            if not res:
                error = 'Machine {} failed internet connection.'.format(unit)
                error_string.append(error)
        if exposed and exposed['fail'] is not ():
            error = ('Application(s) {0} failed expose '
                     'test'.format(exposed['fail']))
            error_string.append(error)
        return error_string

    def reboot_machines(self, client):
        log.info("Starting reboot of all containers.")
        try:
            for machine, m_info in client.get_status().iter_machines():
                cont_ids = []
                try:
                    cont_ids.extend([c['instance-id'] for c in
                                    m_info.get('containers').values()])
                except KeyError:
                    log.info('No containers for machine: {}'.format(machine))
                if cont_ids:
                    log.info('Restarting containers: {0} on '
                             'machine: {1}'.format(cont_ids, machine))
                    self.ssh(client, machine,
                             'sudo lxc restart {}'.format(' '.join(cont_ids)))
                log.info("Restarting machine: {}".format(machine))
                client.juju('run', ('--machine', machine,
                                    'sudo shutdown -r now'))
                hostname = client.get_status().get_machine_dns_name(machine)
                wait_for_port(hostname, 22, timeout=240)

        except subprocess.CalledProcessError as e:
            logging.info(
                "Error running shutdown:\nstdout: %s\nstderr: %s",
                e.output, getattr(e, 'stderr', None))
        client.wait_for_started()

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

    def ping_units(self, client, source, units):
        """Calls out to our subordinate network-health charm to ping targets.

        :param client: The juju client to address
        :param source: The specific network-health unit to send from
        :param units: The units to ping
        """
        units = self.to_json(units)
        args = "targets='{}'".format(units)
        action_id = client.action_do(source, 'ping', args)
        result = client.action_fetch(action_id)
        result = yaml.safe_load(result)['results']['results']
        return result

    def is_ipv6(self, address):
        try:
            socket.inet_pton(socket.AF_INET6, address)
        except socket.error:
            return False
        return True

    def to_json(self, units):
        """Returns a formatted json string to be passed through juju run-action.

        :param units: Dict of units
        :return: A "JSON-like" string that can be passed to Juju without it
        puking
        """
        json_string = json.dumps(units, separators=(',', '='))
        # Replace curly brackets so juju doesn't think it's JSON and puke
        json_string = json_string.replace('{', '(')
        json_string = json_string.replace('}', ')')
        return json_string

    def parse_targets(self, status):
        """Returns targets based on supplied juju status information.

        :param apps: Dict of applications via 'juju status --format yaml'
        """
        targets = {}
        for application, units in status.get_applications().items():
            target_units = {}
            if 'units' in units:
                for unit_id, info in units.get('units').items():
                    target_units[unit_id] = info['public-address']
                targets[application] = target_units
        return targets


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test Network Health")
    add_basic_testing_arguments(parser)
    parser.add_argument('--bundle', help='Bundle to test network against')
    parser.add_argument('--model', help='Existing Juju model to test against')
    parser.add_argument('--reboot', type=bool,
                        help='Reboot machines and re-run tests, default=False')
    parser.set_defaults(reboot=False)
    parser.set_defaults(series='trusty')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    test = AssessNetworkHealth(args)
    if args.model is None:
        bs_manager = BootstrapManager.from_args(args)
        with bs_manager.booted_context(args.upload_tools):
            test.assess_network_health(bs_manager.client, bundle=args.bundle,
                                       series=args.series, reboot=args.reboot)
    else:
        client = client_for_existing(args.juju_bin,
                                     os.environ['JUJU_HOME'])
        try:
            test.assess_network_health(client, bundle=args.bundle,
                                       target_model=args.model,
                                       series=args.series,
                                       reboot=args.reboot)
        finally:
            test.cleanup(client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
