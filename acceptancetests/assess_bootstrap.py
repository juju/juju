#!/usr/bin/env python3
from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
import logging
import sys

from deploy_stack import (
    BootstrapManager,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    temp_dir,
    )


log = logging.getLogger("assess_bootstrap")


INVALID_URL = 'example.com/invalid'


@contextmanager
def thin_booted_context(bs_manager, **kwargs):
    client = bs_manager.client
    with bs_manager.top_context() as machines:
        with bs_manager.bootstrap_context(machines):
            client.kill_controller()
            client.bootstrap(**kwargs)
        with bs_manager.runtime_context(machines):
            yield client


def assess_base_bootstrap(bs_manager):
    with thin_booted_context(bs_manager) as client:
        client.get_status(1)
        log.info('Environment successfully bootstrapped.')


def prepare_metadata(client, local_dir, agent_stream=None, source=None):
    """Fill the given directory with metadata using sync_tools."""
    client.sync_tools(local_dir, agent_stream, source)


@contextmanager
def prepare_temp_metadata(client, source_dir=None, agent_stream=None,
                          source=None):
    """Fill a temporary directory with metadata using sync_tools."""
    if source_dir is not None:
        yield source_dir
    else:
        with temp_dir() as md_dir:
            prepare_metadata(client, md_dir, agent_stream, source)
            yield md_dir


def assess_metadata(bs_manager, local_source):
    client = bs_manager.client
    # This disconnects from the metadata source, as INVALID_URL is different.
    # agent-metadata-url | tools-metadata-url
    client.env.update_config({'agent-metadata-url': INVALID_URL})
    with prepare_temp_metadata(client, local_source) as metadata_dir:
        log.info('Metadata written to: {}'.format(metadata_dir))
        with thin_booted_context(bs_manager,
                                 metadata_source=metadata_dir):
            log.info('Metadata bootstrap successful.')
            data = client.get_model_config()
    if INVALID_URL != data['agent-metadata-url']['value']:
        raise JujuAssertionError('Error, possible web metadata.')


def get_controller_hostname(client):
    """Get the hostname of the controller for this model."""
    controller_client = client.get_controller_client()
    name = controller_client.run(['hostname'], machines=['0'], use_json=False)
    return name.strip()


def assess_to(bs_manager, to):
    """Assess bootstrapping with the --to option."""
    if to is None:
        raise ValueError('--to not given when testing to')
    with thin_booted_context(bs_manager) as client:
        log.info('To {} bootstrap successful.'.format(to))
        addr = get_controller_hostname(client)
    if addr != to:
        raise JujuAssertionError(
            'Not bootstrapped to the correct address; expected {}, got {}'.format(to, addr))


def assess_bootstrap(args):
    bs_manager = BootstrapManager.from_args(args)
    if 'base' == args.part:
        assess_base_bootstrap(bs_manager)
    elif 'metadata' == args.part:
        assess_metadata(bs_manager, args.local_metadata_source)
    elif 'to' == args.part:
        assess_to(bs_manager, args.to)


def parse_args(argv=None):
    """Parse all arguments.

    In addition to the basic testing arguments this script also accepts:
    part: The first argument, which is the name of test part to run.
    --local-metadata-source: If given it should be a directory that contains
    the agent to use in the test. This skips downloading them."""
    parser = ArgumentParser(description='Test the bootstrap command.')
    parser.add_argument('part', choices=['base', 'metadata', 'to'],
                        help='Which part of bootstrap to assess')
    add_basic_testing_arguments(parser)
    parser.add_argument('--local-metadata-source',
                        action='store', default=None,
                        help='Directory with pre-loaded metadata.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    assess_bootstrap(args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
