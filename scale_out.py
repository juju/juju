#!/usr/bin/env python
from argparse import ArgumentParser
from contextlib import contextmanager
import logging
import re

from deploy_stack import (
    boot_context,
)
from jujupy import (
    client_from_config,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


logger = logging.getLogger("scale_out")


def parse_args(argv=None):
    """Parse command line args into an args object."""
    parser = ArgumentParser()
    add_basic_testing_arguments(parser)
    parser.add_argument('charms', nargs="+", help='Charms to deploy.')
    return parser.parse_args(argv)


@contextmanager
def scaleout_setup(args):
    """Setup and bootstrap the Juju environment."""
    logger.info("Bootstrapping the scaleout test environment.")
    series = args.series
    if series is None:
        series = 'trusty'
    client = client_from_config(args.env, args.juju_bin, args.debug,
                                soft_deadline=args.deadline)
    with boot_context(
            args.temp_env_name,
            client,
            args.bootstrap_host,
            args.machine,
            series,
            args.agent_url,
            args.agent_stream,
            args.logs, args.keep_env,
            args.upload_tools,
            region=args.region,
            ):
        if args.machine is not None:
            client.add_ssh_machines(args.machine)
        yield client


def get_service_name(charm):
    """Return a service name derived from a charm URL."""
    return re.split('[:/]', charm).pop()


def deploy_charms(client, charms):
    """Deploy each of the given charms."""
    for charm in charms:
        logger.info("Deploying %s.", charm)
        service_name = get_service_name(charm)
        client.deploy(charm, service=service_name)
    client.wait_for_started()


def scale_out(client, charm, num_units=5):
    """Use Juju add-unit to scale out the given charm."""
    service_name = get_service_name(charm)
    logger.info("Adding %d units to %s.", num_units, service_name)
    client.juju('add-unit', (service_name, '-n', str(num_units)))
    client.wait_for_started()


def main():
    """ Test Juju scale out."""
    args = parse_args()
    configure_logging(args.verbose)
    charms = args.charms
    with scaleout_setup(args) as client:
        deploy_charms(client, charms)
        for charm in charms:
            scale_out(client, charm)

if __name__ == '__main__':
    main()
