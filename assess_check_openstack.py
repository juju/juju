#!/usr/bin/env python
"""
    python assess_check_openstack.py --image
    843bef24-8c70-40e1-a380-972717c9e4b3
    4475ef24-8c70-40e1-a380-972717c9e4b3
    --region India USA
    --series xenial trusty
    --arch amd64
    --dest /home/viswesn/openstack/
    --endpoint http://10.0.2.5:7000/v2  http://10.0.2.75:6000/v2
"""

from __future__ import print_function

import argparse
import logging
import sys
import json
import subprocess
import os

from deploy_stack import (
    BootstrapManager,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    temp_dir,
    )

from assess_agent_metadata import (
    append_empty_string,
    )

__metaclass__ = type

log = logging.getLogger("access_check_openstack")

JUJU_IMAGE_INDEX = "/images/streams/v1/index.json"
JUJU_TOOL_INDEX = "/tools/streams/v1/index2.json"


def get_openstack_config(args):
    do_action = len(args.image)
    try:
        for x in range(do_action):
            series = (args.series[x] if x < len(args.series)
                      else args.series[0]) if (type(args.series) == list)\
                else args.series

            arch = (args.arch[x] if x < len(args.arch)
                    else args.arch[0]) if (type(args.arch) == list)\
                else args.arch

            stream = (args.stream[x] if x < len(args.stream)
                      else args.stream[0]) if (type(args.stream) == list)\
                else args.stream

            endpoint = (args.endpoint[x] if x < len(args.endpoint)
                        else args.endpoint[0])\
                if (type(args.endpoint) == list) else args.endpoint

            region = (args.region[x] if x < len(args.region)
                      else args.region[0]) if (type(args.region) == list)\
                else args.region

            agent_file = (args.agent_file[x] if x < len(args.agent_file)
                          else args.agent_file[0]) \
                if (type(args.agent_file) == list) else args.agent_file

            yield dict(
                series=series,
                arch=arch,
                stream=stream,
                endpoint=endpoint,
                dest=args.dest,
                region=region,
                agent_file=agent_file,
                image=args.image[x]
            )
    except Exception as e:
            logging.exception(e)


def do_validate_image(openstack, client):
    try:
        output = client.validate_images(openstack["region"],
                                        openstack["endpoint"],
                                        openstack["dest"],
                                        openstack["series"],
                                        provider="openstack")
    except subprocess.CalledProcessError as e:
        logging.exception(e)
        raise JujuAssertionError(
            "Failed to execute juju metadata validate-images")

    output_ = json.loads(output)
    if output_["ImageIds"][0] != openstack["image"]:
        log.debug("juju validate-image image: actual {} and expected {}".
                  format(output_["ImageIds"], openstack["image"]))
        raise JujuAssertionError('juju validate-image failed for image')

    if output_["Region"] != openstack["region"]:
        log.debug("juju validate-image region: actual {} and expected {}".
                  format(output_["Region"], openstack["region"]))
        raise JujuAssertionError('juju validate-image failed for region')

    indexURL = "file://" + openstack["dest"] + JUJU_IMAGE_INDEX
    if output_["Resolve Metadata"]["indexURL"] != indexURL:
        log.debug("juju validate-image indexURL: actual {} and expected {}".
                  format(output_["Resolve Metadata"]["indexURL"], indexURL))
        raise JujuAssertionError('juju validate-image failed for region')

    log.info("juju validate-image done successfully")


def do_validate_tool(openstack, client):
    try:
        output = client.validate_tools(openstack["region"],
                                       openstack["endpoint"],
                                       openstack["dest"],
                                       openstack["series"],
                                       openstack["stream"],
                                       provider="openstack")
        output_ = json.loads(output)
        indexURL = "file://" + openstack["dest"] + JUJU_TOOL_INDEX
        if output_["Resolve Metadata"]["indexURL"] != indexURL:
            log.debug("juju validate-image indexURL actual {} and expected {}".
                      format(output_["Resolve Metadata"]["indexURL"],
                             indexURL))
            raise JujuAssertionError('juju validate-image failed for region')
    except subprocess.CalledProcessError as e:
        logging.exception(e)
        raise JujuAssertionError(
            "Failed to execute juju metadata validate-tools")

    log.info("juju validate-tools done successfully")


def copy_agent_file(openstack):
    stream = openstack["stream"]
    src = openstack["agent_file"]

    if stream.startswith("releas"):
        agent_dir = openstack["dest"] + "/tools/released/"
    else:
        agent_dir = openstack["dest"] + "/tools/develop/"

    if not os.path.exists(agent_dir):
        os.makedirs(agent_dir)

    dst = agent_dir + os.path.basename(openstack["agent_file"])
    append_empty_string(src, dst)
    return


def do_access_check_openstack(openstack, client):
    copy_agent_file(openstack)
    client.generate_image(openstack["image"],
                          openstack["arch"],
                          openstack["series"],
                          openstack["region"],
                          openstack["endpoint"],
                          openstack["dest"])

    client.generate_tool(openstack["dest"],
                         openstack["stream"])

    do_validate_image(openstack, client)
    do_validate_tool(openstack, client)
    return


def access_check_openstack(args):
    openstacks = get_openstack_config(args)

    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    for openstack in openstacks:
        do_access_check_openstack(openstack, client)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Perform juju metadata validation for images and tools")
    add_basic_testing_arguments(parser)

    parser.add_argument('--arch', nargs='*',
                        default='amd64',
                        help='the image architecture')
    parser.add_argument('--stream', nargs='*',
                        default='released',
                        help='the image stream')
    parser.add_argument('--endpoint', required=True, nargs='*',
                        default=None,
                        help='the regional cloud endpoint')
    parser.add_argument('--image', nargs='*',
                        default=None,
                        help='the image id')
    parser.add_argument('--agent-file', nargs='*',
                        action='store', default=None,
                        help='agent file to be used during bootstrap.')

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    args.debug = True
    args.verbose = logging.DEBUG
    configure_logging(args.verbose)

    with temp_dir() as args.dest:
        access_check_openstack(args)

    return 0

if __name__ == '__main__':
    sys.exit(main())
