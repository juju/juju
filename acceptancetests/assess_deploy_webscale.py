#!/usr/bin/env python3
""" Test webscale deployment

    1. deploying kubenetes core and asserting it is `healthy`
"""

from __future__ import print_function

import argparse
import logging
import sys
import os
import subprocess

import requests

from deploy_stack import (
    BootstrapManager,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
)

from jujucharm import (
    local_charm_path,
)
from jujupy.utility import until_timeout

__metaclass__ = type

log = logging.getLogger("assess_deploy_webscale")

def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Webscale charm deployment CI test")
    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)

def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client
    return 0

if __name__ == '__main__':
    sys.exit(main())