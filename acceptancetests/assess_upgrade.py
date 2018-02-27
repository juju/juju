#!/usr/bin/env python
""" Assess upgrading juju controllers and models.

Bootstrap a previous version of juju and then upgrade the controller and models
to the new one.

  - Spins up a local streams server (requires to be run on lxd or on a machine
     with an externally routable interface.
  - Bootstraps using the 'stable' juju then upgrades to the 'devel' version.
  - Deploys mediawiki and mysql to ensure workloads continue working.
"""

from __future__ import print_function

import argparse
import logging
import sys

from collections import (
    namedtuple,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
    )
from jujupy.binaries import (
    get_stable_juju
    )
from jujupy.client import (
    ModelClient,
    get_stripped_version_number,
)
from jujupy.stream_server import (
    StreamServer,
    agent_tgz_from_juju_binary,
    )
from jujupy.wait_condition import (
    WaitModelVersion,
    wait_until_model_upgrades,
)
from jujupy.workloads import (
    deploy_mediawiki_with_db,
    assert_mediawiki_is_responding
    )


__metaclass__ = type


log = logging.getLogger("assess_upgrade")


VersionParts = namedtuple('VersionParts', ['version', 'series', 'arch'])


def assess_upgrade_from_stable_to_develop(args, stable_bsm, devel_client):
    stable_client = stable_bsm.client
    with temp_dir() as base_dir:
        stream_server = StreamServer(base_dir)
        setup_agent_metadata(
            stream_server, args.stable_juju_agent,
            stable_client, base_dir, 'proposed')
        setup_agent_metadata(
            stream_server, args.devel_juju_agent,
            devel_client, base_dir, 'proposed')
        with stream_server.server() as url:
            stable_client.env.update_config({
                'agent-metadata-url': url,
                'agent-stream': 'proposed'
            })
            with stable_bsm.booted_context(False):
                assert_stable_model_is_correct(stable_client)

                deploy_mediawiki_with_db(stable_client)
                assert_mediawiki_is_responding(stable_client)
                upgrade_stable_to_devel_version(devel_client)
                assert_mediawiki_is_responding(devel_client)


def upgrade_stable_to_devel_version(client):
    devel_version = get_stripped_version_number(client.version)
    client.get_controller_client().juju('upgrade-juju', ('-m', 'controller'))
    assert_model_is_version(client.get_controller_client(), devel_version)
    wait_until_model_upgrades(client)

    client.juju('upgrade-juju', ())
    assert_model_is_version(client, devel_version)
    wait_until_model_upgrades(client)


def assert_stable_model_is_correct(stable_client):
    assert_model_is_version(
        stable_client.get_controller_client(),
        get_stripped_version_number(stable_client.version))
    assert_model_is_version(
        stable_client,
        get_stripped_version_number(stable_client.version))


def assert_model_is_version(client, expected_version, timeout=600):
    # Check agent & machine version
    client.wait_for_version(expected_version, timeout)
    # Also check model version
    client.wait_for(WaitModelVersion(expected_version, timeout))


def setup_agent_metadata(stream_server, agent, client, tmp_dir, stream):
    version_parts = get_version_parts(client.version)

    if agent is None:
        agent_details = agent_tgz_from_juju_binary(client.full_path, tmp_dir)
    else:
        agent_details = agent

    stream_server.add_product(
        stream,
        version_parts.version,
        version_parts.arch,
        client.env.get_option('default-series'),
        agent_details)
    # Trusty needed for wikimedia charm.
    stream_server.add_product(
        stream,
        version_parts.version,
        version_parts.arch,
        'trusty',
        agent_details)


def get_version_parts(version_string):
    parts = version_string.split('-')
    return VersionParts(parts[0], parts[1], parts[2])


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Test juju upgrades for controllers and models')
    add_basic_testing_arguments(parser, existing=False)
    parser.add_argument(
        '--stable-juju-bin',
        help='Path to juju binary to be used as the stable version of juju.')
    parser.add_argument(
        '--stable-juju-agent',
        help='Path to agent to use when bootstrapping with stable binary'
    )
    parser.add_argument(
        '--devel-juju-agent',
        help='Path to agent to use when bootstrapping with stable binary'
    )
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    stable_bsm = get_stable_juju(args, args.stable_juju_bin)
    devel_client = stable_bsm.client.clone(
        version=ModelClient.get_version(args.juju_bin),
        full_path=args.juju_bin,
        debug=args.debug
    )

    assess_upgrade_from_stable_to_develop(args, stable_bsm, devel_client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
