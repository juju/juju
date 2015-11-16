#!/usr/bin/env python
from argparse import ArgumentParser
from contextlib import contextmanager
import logging

from deploy_stack import (
    boot_context,
)
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
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
    parser.add_argument('charms', help='Charms to deploy.')
    return parser.parse_args(argv)


@contextmanager
def scaleout_setup(args):
    """Setup and bootstrap the Juju environment."""
    logger.info("Bootstrapping the scaleout test environment.")
    series = args.series
    if series is None:
        series = 'trusty'
    client = EnvJujuClient.by_version(
        SimpleEnvironment.from_config(args.env), args.juju_bin, args.debug)
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
            permanent=False,
            region=args.region,
            ):
        if args.machine is not None:
            client.add_ssh_machines(args.machine)
        yield client


def deploy_charms(client, charms):
    """Deploy each of the given charms."""
    for charm in charms:
        logger.info("Deploying %s.", charm)
        client.deploy(charm)
    client.wait_for_started()


def scale_out(client, charm, num_units=5):
    """Use Juju add-unit to scale out the given charm."""
    logger.info("Adding %d units to %s.", num_units, charm)
    client.juju('add-unit', (charm, '-n', str(num_units)))
    client.wait_for_started()


def main():
    """ Test Juju scale out."""
    args = parse_args()
    configure_logging(args.verbose)
    charms = args.charms.split()
    with scaleout_setup(args) as client:
        deploy_charms(client, charms)
        for charm in charms:
            scale_out(client, charm)

if __name__ == '__main__':
    main()
