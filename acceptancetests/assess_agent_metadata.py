#!/usr/bin/env python3
"""
 Juju agent-metadata-url validation on passing as an argument during
 juju bootstrap

 Juju agent-metadata-url validation using cloud definition yaml file to
 verify that agent-metadata-url specified in the yaml file is applied
 while running juju boostrap command.

 Usage: python assess_agent_metadata.py --agent-file=/path/to/juju-*.tgz

 Example: python assess_agent_metadata.py
                --agent-file=/home/juju/juju-2.0.1-xenial-amd64.tgz
"""

from __future__ import print_function

from argparse import ArgumentParser
from contextlib import contextmanager
from hashlib import sha256

import logging
import os
import subprocess
import sys
import json

from deploy_stack import (
    BootstrapManager,
    )

from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    temp_dir,
    temp_yaml_file,
)
from remote import (
    remote_from_unit,
    remote_from_address,
)
from jujucharm import (
    local_charm_path,
)

log = logging.getLogger("assess_agent_metadata")


def get_sha256_sum(filename):
    """
    Get SHA256 sum of the given filename
    :param filename: A string representing the filename to operate on
    """
    with open(filename, 'rb') as infile:
        return sha256(infile.read()).hexdigest()


def assert_cloud_details_are_correct(client, cloud_name, example_cloud):
    """
    Check juju add-cloud is performed successfully and it is available
    in the juju client.
    :param client: The juju client
    :param cloud_name: The name of the cloud added
    :param example_cloud: The content of the cloud
    """
    clouds = client.env.read_clouds()
    try:
        if clouds['clouds'][cloud_name] != example_cloud:
            raise JujuAssertionError('Cloud mismatch')
    except KeyError:
        raise JujuAssertionError('Cloud missing {}'.format(cloud_name))


def get_local_url_and_sha256(agent_dir, controller_url, agent_stream):
    """
    Get the agent URL (local file location: file:///) and SHA256
    of the agent-file passed
    :param agent_dir: The top level directory location of agent file.
    :param controller_url: The controller used agent file url
    :param agent_stream: String representing agent stream name
    """
    local_url = os.path.join(agent_dir, "tools", agent_stream,
                             os.path.basename(controller_url))

    local_sha256 = get_sha256_sum(local_url)
    local_file_path = "file://" + local_url
    return local_file_path, local_sha256


def get_controller_url_and_sha256(client):
    """
    Get the agent url and sha256 of the launched client
    :param client: Juju client
    """
    controller_client = client.get_controller_client()
    output = controller_client.run(
        ['cat /var/lib/juju/tools/machine-0/downloaded-tools.txt'],
        machines=['0'])
    stdout_details = json.loads(output[0]['Stdout'])
    return stdout_details['url'], stdout_details['sha256']


def assert_metadata_is_correct(expected_agent_metadata_url, client):
    """
    verify the client agent-metadata-url matches the specified value
    :param expected_agent_metadata_url: The expected agent file path.
    :param client: Juju client
    """
    data = client.get_model_config()
    actual_agent_metadata_url = data['agent-metadata-url']['value']
    if expected_agent_metadata_url != actual_agent_metadata_url:
        raise JujuAssertionError(
            'agent-metadata-url mismatch. Expected: {} Got: {}'.format(
                expected_agent_metadata_url, actual_agent_metadata_url))

    log.info('bootstrap successfully with agent-metadata-url={}'.format(
        actual_agent_metadata_url))


def verify_deployed_charm(client, remote, unit="0"):
    """Verify the deployed charm

    Make sure deployed charm uses the same juju tool of the
    controller by verifying the sha256 sum

    :param client: Juju client
    :param remote: The remote object of the deployed charm
    :param unit: String representation of deployed charm unit.
    """
    output = remote.cat(
        "/var/lib/juju/tools/machine-{}/downloaded-tools.txt".format(unit))

    deserialized_output = json.loads(output)
    _, controller_sha256 = get_controller_url_and_sha256(client)

    if deserialized_output['sha256'] != controller_sha256:
        raise JujuAssertionError(
            'agent-metadata-url mismatch. Expected: {} Got: {}'.format(
                controller_sha256, deserialized_output))

    log.info("Charm verification done successfully")


def deploy_machine_and_verify(client, series="bionic"):
    """Deploy machine using juju add-machine of specified series
    and verify that it make use of same agent-file used by the
    controller.

    :param client: Juju client
    :param series: The charm series to deploy
    """
    old_status = client.get_status()
    client.juju('add-machine', ('--series', series))
    new_status = client.wait_for_started()

    # This will iterate only once because we just added only one
    # machine after old_status to new_status.
    for unit, machine in new_status.iter_new_machines(old_status):
        hostname = machine.get('dns-name')
        if hostname:
            remote = remote_from_address(hostname, machine.get('series'))
            verify_deployed_charm(client, remote, unit)
        else:
            raise JujuAssertionError(
                'Unable to get information about added machine')

    log.info("add-machine verification done successfully")


def deploy_charm_and_verify(client, series="bionic", charm_app="dummy-source"):
    """
    Deploy dummy charm from local repository and
    verify it uses the specified agent-metadata-url option
    :param client: Juju client
    :param series: The charm series to deploy
    :param charm_app: Juju charm application
    """
    charm_source = local_charm_path(
        charm=charm_app, juju_ver=client.version, series=series)
    client.deploy(charm_source)
    client.wait_for_started()
    client.set_config(charm_app, {'token': 'one'})
    client.wait_for_workloads()
    remote = remote_from_unit(client, "{}/0".format(charm_app))
    verify_deployed_charm(client, remote)
    log.info(
        "Successfully deployed charm {} of series {} and verified".format(
            "dummy-source", series))


def verify_deployed_tool(agent_dir, client, agent_stream):
    """
    Verify the bootstrapped controller makes use of the the specified
    agent-metadata-url.
    :param agent_dir:  The top level directory location of agent file.
    :param client: Juju client
    :param agent_stream: String representing agent stream name
    """
    controller_url, controller_sha256 = get_controller_url_and_sha256(client)

    local_url, local_sha256 = get_local_url_and_sha256(
        agent_dir, controller_url, agent_stream)

    if local_url != controller_url:
        raise JujuAssertionError(
            "mismatch local URL {} and controller URL {}".format(
                local_url, controller_url))

    if local_sha256 != controller_sha256:
        raise JujuAssertionError(
            "mismatch local SHA256 {} and controller SHA256 {}".format(
                local_sha256, controller_sha256))


def set_new_log_dir(bs_manager, dir_name):
    log_dir = os.path.join(bs_manager.log_dir, dir_name)
    os.mkdir(log_dir)
    bs_manager.log_dir = log_dir


def get_controller_series_and_alternative_series(client):
    """Get controller and alternative controller series

    Returns the series used by the controller and an alternative series
    that is not used by the controller.

    :param client: The juju client
    :return: controller and non-controller series
    """
    supported_series = ['bionic', 'xenial', 'trusty', 'zesty']
    controller_status = client.get_status(controller=True)
    machines = dict(controller_status.iter_machines())
    controller_series = machines['0']['series']
    try:
        supported_series.remove(controller_series)
    except:
        raise ValueError("Unknown series {}".format(controller_series))
    return controller_series, supported_series[0]


def assess_metadata(args, agent_dir, agent_stream):
    """
    Bootstrap juju controller with agent-metadata-url value
    and verify that bootstrapped controller makes use of specified
    agent-metadata-url value.
    :param args: Parsed command line arguments
    :param agent_dir: The top level directory location of agent file.
    :param agent_stream: String representing agent stream name
    """
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    agent_metadata_url = os.path.join(agent_dir, "tools")

    client.env.discard_option('tools-metadata-url')
    client.env.update_config(
        {
            'agent-metadata-url': agent_metadata_url,
            'agent-stream': agent_stream
        }
    )
    log.info('bootstrap to use --agent_metadata_url={}'.format(
        agent_metadata_url))
    client.generate_tool(agent_dir, agent_stream)
    set_new_log_dir(bs_manager, "assess_metadata")

    with bs_manager.booted_context(args.upload_tools):
        log.info('Metadata bootstrap successful.')
        assert_metadata_is_correct(agent_metadata_url, client)
        verify_deployed_tool(agent_dir, client, agent_stream)
        log.info("Successfully deployed and verified agent-metadata-url")
        series_details = get_controller_series_and_alternative_series(client)
        controller_series, alt_controller_series = series_details
        deploy_charm_and_verify(client, controller_series, "dummy-source")
        deploy_machine_and_verify(client, alt_controller_series)


def get_cloud_details(client, agent_metadata_url, agent_stream):
    """
    Create a cloud detail content to be used during add-cloud.
    :param client: Juju client
    :param agent_metadata_url: Sting representing agent-metadata-url
    :param agent_stream: String representing agent stream name
    """
    cloud_name = client.env.get_cloud()
    cloud_details = {
        'clouds': {
            cloud_name: {
                'type': client.env.provider,
                'regions': {client.env.get_region(): {}},
                'config': {
                    'agent-metadata-url': 'file://{}'.format(
                        agent_metadata_url),
                    'agent-stream': agent_stream,
                }
            }
        }
    }
    return cloud_details


def assess_add_cloud(args, agent_dir, agent_stream):
    """
    Perform juju add-cloud by creating a yaml file for cloud
    with agent-metadata-url value and bootstrap the juju environment.
    :param args: Parsed command line arguments
    :param agent_dir: The top level directory location of agent file.
    :param agent_stream: String representing agent stream name
    """

    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    agent_metadata_url = os.path.join(agent_dir, "tools")
    # Remove the tool metadata url from the config (note the name, is
    # for historic reasons)
    client.env.discard_option('tools-metadata-url')
    cloud_details = get_cloud_details(client, agent_metadata_url, agent_stream)

    with temp_yaml_file(cloud_details) as new_cloud:
        cloud_name = client.env.get_cloud()
        client.add_cloud(cloud_name, new_cloud)
        # Need to make sure we've refreshed any cache that we might have (as
        # this gets written to file during the bootstrap process.
        client.env.load_yaml()
        clouds = cloud_details['clouds'][cloud_name]
        assert_cloud_details_are_correct(client, cloud_name, clouds)

    client.generate_tool(agent_dir, agent_stream)
    set_new_log_dir(bs_manager, "assess_add_cloud")

    with bs_manager.booted_context(args.upload_tools):
        log.info('Metadata bootstrap successful.')
        verify_deployed_tool(agent_dir, client, agent_stream)
        log.info("Successfully deployed and verified add-cloud")
        deploy_charm_and_verify(client, "bionic", "dummy-source")
        log.info("Successfully deployed charm and verified")


def clone_tgz_file_and_change_shasum(original_tgz_file, new_tgz_file):
    """
    Create a new tgz file from the original tgz file and then add empty file
    to it so that the sha256 sum of the new tgz file will be different from
    that of original tgz file. We use this to make sure that controller
    deployed on bootstrap used of the new tgz file.
    :param original_tgz_file: The source tgz file
    :param new_tgz_file: The destination tgz file
    """
    if not original_tgz_file.endswith(".tgz"):
        raise Exception("{} is not tgz file".format(original_tgz_file))
    try:
        command = "cat {}  <(echo -n ''| gzip)> {}".format(
            original_tgz_file, new_tgz_file)
        subprocess.Popen(command, shell=True, executable='/bin/bash')
    except subprocess.CalledProcessError as e:
        raise Exception("Failed to create a tool file {} - {}".format(
            original_tgz_file, e))


@contextmanager
def make_unique_tool(agent_file, agent_stream):
    """
    Create a tool directory for juju agent tools and stream and invoke
    clone_tgz_file_and_change_shasum function for the agent-file passed.
    :param agent_file: The agent file to use
    :param agent_stream: String representing agent stream name
    """
    with temp_dir() as agent_dir:
        juju_tool_src = os.path.join(agent_dir, "tools", agent_stream)
        os.makedirs(juju_tool_src)
        clone_tgz_file_and_change_shasum(agent_file, os.path.join(
            juju_tool_src, os.path.basename(agent_file)))
        log.debug("Created agent directory to perform bootstrap".format(
            agent_dir))
        yield agent_dir


def parse_args(argv):
    """Parse all arguments."""
    parser = ArgumentParser(
        description="Test bootstrap for agent-metdadata-url")

    add_basic_testing_arguments(parser, existing=False)

    parser.add_argument('--agent-file', required=True, action='store',
                        help='agent file to be used during bootstrap.')

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    if not os.path.isfile(args.agent_file):
        raise Exception(
            "Unable to find juju agent file {}".format(args.agent_file))

    agent_stream = args.agent_stream if args.agent_stream else 'testing'

    with make_unique_tool(args.agent_file, agent_stream) as agent_dir:
        assess_metadata(args, agent_dir, agent_stream)

    with make_unique_tool(args.agent_file, agent_stream) as agent_dir:
        assess_add_cloud(args, agent_dir, agent_stream)

    return 0


if __name__ == '__main__':
    sys.exit(main())
