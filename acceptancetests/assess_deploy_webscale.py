#!/usr/bin/env python3
""" Test webscale deployment

    1. deploying kubernetes core and asserting it is `healthy`
    2. inspect the logs to parse timings from trace logs
    3. send timings to the reporting client
       a. include charm revisions in the tags
"""

from __future__ import print_function

import argparse
import logging
import sys
import os
import subprocess
import re
import requests
import functools
import time

from deploy_stack import (
    BootstrapManager,
    deploy_caas_stack,
    deploy_iaas_stack,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    get_current_model,
)

from jujucharm import (
    local_charm_path,
)
from jujupy.utility import until_timeout
from reporting import (
    construct_metrics,
    get_reporting_client,
)

__metaclass__ = type

log = logging.getLogger("assess_deploy_webscale")

def deploy_bundle(client, charm_bundle, stack_type):
    """Deploy the given charm bundle

    :param client: Jujupy ModelClient object
    :param charm_bundle: Optional charm bundle string
    """
    bundle = None
    if not charm_bundle:
        bundle = local_charm_path(
            charm='bundles-kubernetes-core-lxd.yaml',
            repository=os.environ['JUJU_REPOSITORY'],
            juju_ver=client.version,
        )
    else:
        bundle = charm_bundle

    stack_client = get_stack_client(stack_type,
        path=bundle,
        client=client,
        charm=(not not charm_bundle),
        timeout=43200,
    )

    if not stack_client.is_cluster_healthy:
        raise JujuAssertionError('cluster is not healthy')

def extract_module_logs(client, module):
    """Extract the logs from destination module.

    :param module: string containing the information to extract from the destination module.
    """
    deploy_logs = client.get_juju_output(
        'debug-log', '-m', 'controller',
        '--no-tail', '--replay', '-l', 'TRACE',
        '--include-module', module,
    )
    return deploy_logs.decode('utf-8')

def extract_txn_timings(logs, module):
    """Extract the transaction timings (txn) from the deploy logs.

    It's expected that the timings are in seconds to 3 decimal places ("0.042s")

    :param logs: string containing all the logs from the module
    :param module: string containing the destination module.
    """
    exp = re.compile(r'{} ran transaction in (?P<seconds>\d+\.\d+)s'.format(module), re.IGNORECASE)
    timings = []
    for timing in exp.finditer(logs):
        timings.append(timing.group("seconds"))
    return list(map(float, timings))

def calculate_total_time(timings):
    """Accumulate transaction timings (txn) from the timings.

    :param timings: expects timings to be floats
    """
    return functools.reduce(lambda x, y: x + y, timings)

def calculate_max_time(timings):
    """Calculate maximum transaction timing from (txn).

    :param timings: expects timings to be floats
    """
    return functools.reduce(lambda x, y: x if x > y else y, timings)

def extract_charm_urls(client):
    """Extract the bundle with revisions
    """
    status = client.get_status()
    application_info = status.get_applications()
    charms = []
    for charm in application_info.values():
        charms.append(charm["charm"])
    return charms

def get_stack_client(stack_type, path, client, timeout=3600, charm=False):
    """Get the stack client dependant on the type of stack we want to deploy on
    """
    if stack_type == "iaas":
        fn = deploy_iaas_stack
    elif stack_type == "caas":
        fn = deploy_caas_stack
    else:
        raise JujuAssertionError('invalid stack type {}'.format(stack_type))
    return fn(path, client, timeout=timeout, charm=charm)

def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Webscale charm deployment CI test")
    parser.add_argument(
        '--charm-bundle',
        help="Override the charm bundle to deploy",
    )
    parser.add_argument(
        '--logging-module',
        help="Override default module to extract",
        default="juju.state.txn",
    )
    parser.add_argument(
        '--git-sha',
        help="Help the reporting metrics by supplying a git SHA",
        default="",
    )
    parser.add_argument(
        '--reporting-uri',
        help="Reporting uri for sending the metrics to.",
        default="http://root:root@localhost:8086",
    )
    parser.add_argument(
        '--stack-type',
        help="Stack type to use when deploying <iaas|caas>",
        default="caas",
    )
    add_basic_testing_arguments(parser, existing=False)
    # Override the default logging_config default value set by adding basic
    # testing arguments. This way we can have a default value for all tests,
    # then override it again just for this test.
    parser.set_defaults(logging_config="juju.state.txn=TRACE;<root>=INFO;unit=INFO")
    return parser.parse_args(argv)

def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    begin = time.time()
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client
        deploy_bundle(client,
            charm_bundle=args.charm_bundle,
            stack_type=args.stack_type,
        )
        raw_logs = extract_module_logs(client, module=args.logging_module)
        timings = extract_txn_timings(raw_logs, module=args.logging_module)

        # Calculate the timings to forward to the datastore
        metrics = construct_metrics(
            calculate_total_time(timings),
            len(timings),
            calculate_max_time(timings),
            (time.time() - begin),
        )
        log.info("Metrics for deployment: {}".format(metrics))

        # Extract the charm bundle and revision numbers
        charm_urls = ",".join(extract_charm_urls(client))

        try:
            rclient = get_reporting_client(args.reporting_uri)
            rclient.report(metrics, tags={
                "git-sha": args.git_sha,
                "charm-bundle": args.charm_bundle,
                "charm-urls": charm_urls,
            })
        except:
            raise JujuAssertionError("Error reporting metrics")
        log.info("Metrics successfully sent to report storage")
    return 0

if __name__ == '__main__':
    sys.exit(main())
