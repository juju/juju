#!/usr/bin/env python
""" Test Export Bundle functionality.

  - Exporting current model configuration feature is verified.
  - Deploy mediawikisimple bundle.
  - verify if the bundle is created and the content of the file as well by deploying to another model.
"""

from __future__ import print_function

import argparse
import logging
import sys
import os
import filecmp

from deploy_stack import (
    BootstrapManager,
)
from jujucharm import local_charm_path
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    add_model
)

__metaclass__ = type
model = "mymodel"
newmodel = "newmodel"

log = logging.getLogger("assess_bundle_export")

def assess_bundle_export(client, args):
    bundle_source = local_charm_path('mediawiki-simple.yaml',
                                     repository=os.environ['JUJU_REPOSITORY'],
                                     juju_ver='2')
    client.deploy(bundle_source)
    client.wait_for_started()
    client.juju('export-bundle', ('--filename', model))
    log.info("Deploying {}".format("mediawiki-simple bundle..."))

    if not os.path.exists(model+".yaml"):
        raise JujuAssertionError('export bundle command failed to create bundle file.')

    new_client = add_model(client)
    new_client.deploy(model+".yaml")
    new_client.wait_for_started()
    new_client.juju('export-bundle', ('--filename', newmodel))
    log.info("Deploying bundle in new model model.yaml")

    #compare the contents of the file.
    if not filecmp.cmp(model+".yaml", newmodel+".yaml"):
        raise JujuAssertionError('bundle files created mismatch error.')

    os.remove(model+".yaml")
    os.remove(newmodel+".yaml")

def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Test the export-bundle functionality.")
    add_basic_testing_arguments(parser)
    parser.add_argument('--filename', help='file to write the model configuration')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_bundle_export(bs_manager.client, args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
