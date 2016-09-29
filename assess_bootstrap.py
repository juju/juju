#!/usr/bin/env python
from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
import logging
import sys

from deploy_stack import (
    BootstrapManager,
    tear_down,
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
    """Minimal boote_context, for checking bootstrap."""
    with bs_manager.top_context() as machines:
        with bs_manager.bootstrap_context(machines):
            tear_down(bs_manager.client, bs_manager.jes_enabled)
            bs_manager.client.bootstrap(**kwargs)
        with bs_manager.runtime_context(machines):
            yield


def assess_base_bootstrap(bs_manager):
    client = bs_manager.client
    with bs_manager.top_context() as machines:
        with bs_manager.bootstrap_context(machines):
            tear_down(client, client.is_jes_enabled())
            client.bootstrap()
        with bs_manager.runtime_context(machines):
            client.get_status(1)
            log.info('Environment successfully bootstrapped.')


def prepare_metadata(client, dest_dir, source_dir=None):
    """Fill the given directory with metadata using sync_tools."""
    args = []
    if source_dir is not None:
        args.extend(['--source', source_dir])
    client.sync_tools('--local-dir', dest_dir, *args)


@contextmanager
def prepare_temp_metadata(client, source_dir=None):
    """Fill a temperary directory with metadata using sync_tools."""
    if source_dir is not None:
        yield source_dir
    else:
        with temp_dir() as md_dir:
            prepare_metadata(client, md_dir, source_dir)
            yield md_dir


def assess_metadata(bs_manager, args):
    client = bs_manager.client
    # This disconnects from the metadata source, as INVALID_URL is different.
    # agent-metadata-url | tools-metadata-url
    client.env.config['agent-metadata-url'] = INVALID_URL
    with prepare_temp_metadata(client,
                               args.local_metadata_source) as metadata_dir:
        log.info('Metadata written to: {}'.format(metadata_dir))
        with bs_manager.top_context() as machines:
            with bs_manager.bootstrap_context(machines):
                tear_down(client, client.is_jes_enabled())
                client.bootstrap(metadata_source=metadata_dir)
            with bs_manager.runtime_context(machines):
                log.info('Metadata bootstrap successful.')
                data = client.get_model_config()
                if INVALID_URL != data['agent-metadata-url']['value']:
                    raise JujuAssertionError('Error, possible web metadata.')


def assess_bootstrap(args):
    bs_manager = BootstrapManager.from_args(args)
    if 'base' == args.part:
        assess_base_bootstrap(bs_manager)
    elif 'metadata' == args.part:
        assess_metadata(bs_manager, args)


def parse_args(argv=None):
    """Parse all arguments.

    In addition to the basic testing arguments this script also accepts
    --local-metadata-source. If given it should be a directory that contains
    the agent to use in the test. This skips downloading them."""
    parser = ArgumentParser(description='Test the bootstrap command.')
    parser.add_argument('part', choices=['base', 'metadata'],
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
