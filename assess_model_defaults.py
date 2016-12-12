#!/usr/bin/env python
"""Assess the model-defaults command."""

from __future__ import print_function

import argparse
import logging
import sys

from deploy_stack import (
    BootstrapManager,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    )


__metaclass__ = type


log = logging.getLogger('assess_model_defaults')


def assemble_model_default(model_key, default, controller=None, regions=None):
    """Create a dict that contrains the formatted model-defaults data."""
    # Ordering in the regions argument is lost.
    defaults = {'default': default}
    if controller is not None:
        defaults['controller'] = controller
    if regions is not None:
        defaults['regions'] = [
            {'name': region, 'value': region_default}
            for (region, region_default) in regions.items()]
    return {model_key: defaults}


def juju_assert_equal(lhs, rhs, msg):
    if lhs != rhs:
        raise JujuAssertionError(msg, lhs, rhs)


def get_new_model_config(client, region=None, model_name=None):
    """Create a new model, get it's config and then destroy it.

    :param client: Client to use create the new model on.
    :param region: If given and not None, create the new model in that region
        otherwise create it in the client region.
    :param model_name: Name of the new model.
    """
    model_name = model_name or 'temp-model'
    new_model = client.add_model(client.env.clone(model_name))
    config_data = new_model.get_model_config()
    new_model.destroy_model()
    return config_data


def assess_model_defaults_controller(client, model_key, value):
    """Check the setting and unsetting of the controller field."""
    base_line = client.get_model_defaults(model_key)
    default = base_line[model_key]['default']

    client.set_model_defaults(model_key, value)
    juju_assert_equal(
        assemble_model_default(model_key, default, str(value)),
        client.get_model_defaults(model_key),
        'model-defaults: Mismatch on setting controller.')

    client.unset_model_defaults(model_key)
    juju_assert_equal(
        base_line, client.get_model_defaults(model_key),
        'model-defaults: Mismatch after resetting controller.')


def assess_model_defaults_region(client, model_key, value,
                                 cloud=None, region=None):
    """Check the setting and unsetting of a region field."""
    base_line = client.get_model_defaults(model_key)
    default = base_line[model_key]['default']

    client.set_model_defaults(model_key, value, cloud, region)
    juju_assert_equal(
        assemble_model_default(model_key, default, None, {region: str(value)}),
        client.get_model_defaults(model_key),
        'model-defaults: Mismatch on setting region.')

    client.unset_model_defaults(model_key, cloud, region)
    juju_assert_equal(
        base_line, client.get_model_defaults(model_key),
        'model-defaults: Mismatch after resetting region.')


def assess_model_defaults(client, other_region):
    log.info('Checking controller model-defaults.')
    assess_model_defaults_controller(
        client, 'automatically-retry-hooks', False)
    log.info('Checking region model-defaults.')
    assess_model_defaults_region(
        client, 'default-series', 'trusty', region='localhost')
    if other_region is not None:
        pass
        # Test on a region not different the one the client is on.


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Assess the model-defaults command.')
    add_basic_testing_arguments(parser)
    parser.add_argument('--other-region',
                        help='Set the model default for a different region.')
    return parser.parse_args(argv)


# Somewhere I think we should check that region != other_region.
def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_model_defaults(bs_manager.client, args.other_region)
    return 0


if __name__ == '__main__':
    sys.exit(main())
