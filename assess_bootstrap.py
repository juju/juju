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
    temp_dir,
    )


log = logging.getLogger("assess_bootstrap")


@contextmanager
def thin_booted_context(bs_manager):
    with bs_manager.top_context() as machines:
        with bs_manager.bootstrap_context(machines):
            tear_down(bs_manager.client, bs_manager.jes_enabled)
            bs_manager.client.bootstrap()
        with bs_manager.runtime_context(machines):
            yield


def assess_bootstrap(args):
    bs_manager = BootstrapManager.from_args(args)
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
    with temp_dir() as md_dir:
        prepare_metadata(client, md_dir, source_dir)
        yield md_dir


def assess_metadata(args):
    bs_manager = BootstrapManager.from_args(args)
    with prepare_temp_metadata(bs_manager.client,
                               args.local_metadata_source) as metadata_dir:
        log.info('Metadata written to: {}'.format(metadata_dir))
        with bs_manager.top_context() as machines:
            # Disconnect from the charm store.
            with bs_manager.bootstrap_context(machines):
                tear_down(bs_manager.client, bs_manager.jes_enabled)
                bs_manager.client.bootstrap(metadata_source=metadata_dir)
            with bs_manager.runtime_context(machines):
                log.info("Metadata bootstrap successful.")


def parse_args(argv=None):
    """Parse all arguments."""
    parser = ArgumentParser(description='Test the bootstrap command.')
    add_basic_testing_arguments(parser)
    parser.add_argument('--local-metadata-source',
                        action='store', default=None,
                        help='Directory with pre-loaded metadata.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    assess_bootstrap(args)
    assess_metadata(args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
