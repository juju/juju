#!/usr/bin/env python3
""" Assess upgrading juju controllers and models.

Bootstrap a previous version of juju and then upgrade the controller and models
to the new one.

  - Spins up a local streams server (requires to be run on lxd or on a machine
     with an externally routable interface.
  - Bootstraps using the 'stable' juju then upgrades to the 'devel' version.
  - Deploys keystone and percona-cluster to ensure
     workloads continue working.
"""

from __future__ import print_function

import argparse
import logging
import re
import sys

from collections import (
    namedtuple,
    )
from textwrap import (
    dedent,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
    )
from deploy_stack import (
    BootstrapManager
)
from jujupy.binaries import (
    get_stable_juju
    )
from jujupy.client import (
    ModelClient,
    get_stripped_version_number,
    get_version_string_parts,
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
    deploy_keystone_with_db,
    assert_keystone_is_responding
    )


__metaclass__ = type


log = logging.getLogger("assess_upgrade")


VersionParts = namedtuple('VersionParts', ['version', 'release', 'arch'])


def assess_upgrade_from_stable_to_develop(args, stable_bsm, devel_client):
    stable_client = stable_bsm.client
    with temp_dir() as base_dir:
        stream_server = StreamServer(base_dir)
        setup_agent_metadata(
            stream_server, args.stable_juju_agent,
            stable_client, base_dir, 'released')
        setup_agent_metadata(
            stream_server, args.devel_juju_agent,
            devel_client, base_dir, 'released')
        with stream_server.server() as url:
            stable_client.env.update_config({
                'agent-metadata-url': url,
                'agent-stream': 'released'
            })
            with stable_bsm.booted_context(False):
                assert_upgrade_is_successful(stable_client, devel_client)


def assess_upgrade_passing_agent_stream(args, devel_client):
    # Use the same devel juju to ensure this feature is supported.
    stable_bsm = BootstrapManager.from_client(args, devel_client)
    stable_client = stable_bsm.client
    devel_version_parts = get_version_parts(devel_client.version)
    forced_devel_client = devel_client.clone(
        version='{vers}-{release}-{arch}'.format(
            vers=increment_version(devel_client.version),
            release=devel_version_parts.release,
            arch=devel_version_parts.arch))
    with temp_dir() as base_dir:
        stream_server = StreamServer(base_dir)
        setup_agent_metadata(
            stream_server, args.devel_juju_agent,
            stable_client, base_dir, 'released')
        setup_agent_metadata(
            stream_server, args.devel_juju_agent,
            forced_devel_client, base_dir, 'devel',
            force_version=True)
        with stream_server.server() as url:
            stable_client.env.update_config({
                'agent-metadata-url': url,
                'agent-stream': 'released'
            })
            with stable_bsm.booted_context(False):
                assert_upgrade_is_successful(
                    stable_client,
                    forced_devel_client,
                    ('--agent-stream', 'devel'))


def assert_upgrade_is_successful(
        stable_client, devel_client, extra_upgrade_args=()):
    if not isinstance(extra_upgrade_args, tuple):
        raise ValueError("extra_upgrade_args must be a tuple")
    assert_stable_model_is_correct(stable_client)

    deploy_keystone_with_db(stable_client)
    assert_keystone_is_responding(stable_client)
    upgrade_stable_to_devel_version(devel_client, extra_upgrade_args)
    assert_keystone_is_responding(devel_client)


def upgrade_stable_to_devel_version(client, extra_args):
    devel_version = get_stripped_version_number(client.version)
    client.get_controller_client().juju(
        'upgrade-juju', ('-m', 'controller', '--debug',
                         '--agent-stream', 'devel',
                         '--agent-version', devel_version,) + extra_args)
    assert_model_is_version(client.get_controller_client(), devel_version)
    wait_until_model_upgrades(client)

    client.juju('upgrade-juju', ('--debug', '--agent-stream',
                                 'devel', '--agent-version', devel_version,) +
                extra_args)
    assert_model_is_version(client, devel_version)
    wait_until_model_upgrades(client)

    client.wait_for_started()
    client.wait_for_workloads()


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


def setup_agent_metadata(
        stream_server, agent, client, tmp_dir, stream, force_version=False):
    version_parts = get_version_parts(client.version)

    if agent is None:
        agent_details = agent_tgz_from_juju_binary(
            client.full_path,
            tmp_dir,
            force_version=version_parts.version if force_version else None)
    else:
        agent_details = agent

    stream_server.add_product(
        stream,
        version_parts.version,
        version_parts.arch,
        agent_details)
    # Trusty needed for wikimedia charm.
    stream_server.add_product(
        stream,
        version_parts.version,
        version_parts.arch,
        agent_details)


def get_version_parts(version_string):
    parts = get_version_string_parts(version_string)
    return VersionParts(parts[0], parts[1], parts[2])


def increment_version(version_string):
    """Increment the patch version of the given version.

    example:
        2.3.7 -> 2.3.8
        2.4-beta1 -> 2.4-beta2
    """
    juju_version = get_stripped_version_number(version_string)
    try:
        major, minor, patch = juju_version.split('.')
        return '{}.{}.{}'.format(major, minor, increment_patch_version(patch))
    except ValueError:
        # Named patch version
        major, minor_patch = juju_version.split('.')
        minor, patch = minor_patch.split('-')
        return '{}.{}-{}'.format(major, minor, increment_patch_version(patch))


def increment_patch_version(patch_version):
    """Increment the patch version of the given version.

    example:
        7 -> 8
        beta1 -> beta2
    """
    try:
        return int(patch_version) + 1
    except ValueError:
        # Named patch version (alpha/beta etc.)
        name, num = re.search(r'(\D+)(\d+)', patch_version).groups()
        return "{name}{num}".format(
            name=name,
            num=int(num) + 1,
        )


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description=dedent("""\
        Test juju upgrades for controllers and models.
        Uses `juju_bin` as the development version of juju to upgrade to."""),
        formatter_class=argparse.RawDescriptionHelpFormatter)
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

    # LP:1742342 Moving from released stream to devel stream doesn't work,
    # because upgrade-juju doesn't honour --agent-stream over the model-config.
    #
    # assess_upgrade_passing_agent_stream(args, devel_client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
