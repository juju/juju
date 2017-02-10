#!/usr/bin/env python

from __future__ import print_function

import argparse
import logging
import sys
import os

from assess_bootstrap import (
    prepare_temp_metadata,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    )
from jujupy import (
    client_from_config,
    ModelClient,
    )

__metaclass__ = type


log = logging.getLogger("assess_juju_sync_tools")


def verify_agent_tools(agent_dir, agent_stream, agent_version):
    file_verified = True
    sync_tool_dir = os.path.join(agent_dir, "tools", agent_stream)
    for agent_file in os.listdir(sync_tool_dir):
        if agent_file.endswith(".tgz"):
            if not agent_file.startswith("juju-{}".format(agent_version)):
                log.debug(agent_file)
                file_verified = False
    if file_verified:
        log.info('juju sync-tool verification done successfully')


def assess_juju_sync_tools(args, agent_stream, agent_version):
    client = client_from_config(args.env, args.juju_bin, False)
    with prepare_temp_metadata(
            client, agent_stream=agent_stream,
            agent_version=agent_version) as agent_dir:
        verify_agent_tools(agent_dir, agent_stream, agent_version)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test to download juju agent using sync-tool and verify")
    add_basic_testing_arguments(parser)
    parser.add_argument('--agent-version', action='store',
                        help='Juju agent version to download.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    if not args.agent_version:
        juju_bin = args.juju_bin
        agent_version = ModelClient.get_version(juju_bin).rsplit('.', 1)[0]
    else:
        agent_version = args.agent_version
    agent_stream = args.agent_stream if args.agent_stream else 'devel'
    assess_juju_sync_tools(args, agent_stream, agent_version)
    return 0


if __name__ == '__main__':
    sys.exit(main())
