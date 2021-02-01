#!/usr/bin/env python3
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


def assemble_model_default(model_key, default, controller_value=None,
                           region_values=None):
    """Create a dict that contains the formatted model-defaults data."""
    # Ordering in the regions argument is lost.
    defaults = {'default': default}
    if controller_value is not None:
        defaults['controller'] = controller_value
    if region_values is not None:
        defaults['regions'] = [
            {'name': region, 'value': region_default}
            for (region, region_default) in region_values.items()]
    return {model_key: defaults}


def juju_assert_equal(lhs, rhs, msg):
    if lhs != rhs:
        raise JujuAssertionError(msg, lhs, rhs)


def get_new_model_config(client, cloud=None, region=None, model_name=None):
    """Create a new model, get it's config and then destroy it.

    :param client: Client to use create the new model on.
    :param region: If given and not None, create the new model in that region
        otherwise create it in the client region.
    :param model_name: Name of the new model.
    """
    if model_name is None:
        model_name = 'temp-model'
    new_env = client.env.clone(model_name)
    cloud_region = None
    if region is not None:
        new_env.set_region(region)
        cloud_region = client.get_cloud_region(cloud, region)
    # Don't use the bootstrap config, we want to check that the default series
    # is inherited from the model defaults correctly and the bootstrap config
    # might override it.
    new_model = client.add_model(new_env, cloud_region,
                                 use_bootstrap_config=False)
    config_data = new_model.get_model_config()
    new_model.destroy_model()
    return config_data


def assess_model_defaults_case(client, model_key, value, expected_default,
                               cloud=None, region=None):
    """Check the setting and unsetting of a region field."""
    base_line = client.get_model_defaults(model_key)

    client.set_model_defaults(model_key, value, cloud, region)
    juju_assert_equal(expected_default, client.get_model_defaults(model_key),
                      'Mismatch after setting model-default.')
    config = get_new_model_config(client, cloud=cloud, region=region)
    juju_assert_equal(value, config[model_key]['value'],
                      'New model did not use the default.')

    client.unset_model_defaults(model_key, cloud, region)
    juju_assert_equal(base_line, client.get_model_defaults(model_key),
                      'Mismatch after resetting model-default.')


def assess_model_defaults_no_region(client, model_key, value):
    """Check the setting and unsetting of the controller field."""
    default = client.get_model_defaults(model_key)[model_key]['default']
    assess_model_defaults_case(
        client, model_key, value,
        assemble_model_default(model_key, default, value))


def assess_model_defaults_region(client, model_key, value,
                                 cloud=None, region=None):
    """Check the setting and unsetting of a region field."""
    default = client.get_model_defaults(model_key)[model_key]['default']
    assess_model_defaults_case(
        client, model_key, value,
        assemble_model_default(model_key, default, None, {region: str(value)}),
        cloud=cloud, region=region)


def assess_model_defaults(client, other_region):
    log.info('Checking controller model-defaults.')
    assess_model_defaults_no_region(
        client, 'automatically-retry-hooks', False)

    cloud = client.env.get_cloud()
    # TODO - we inconsistently use lxd vs localhost.
    # cloud is recorded in controller as 'localhost'
    # but test clients may be set up to use 'lxd', and
    # for this model defaults test, the difference matters
    if cloud == 'lxd':
        cloud = 'localhost'
    region = client.env.get_region()
    if region is not None:
        log.info('Checking region model-defaults.')
        assess_model_defaults_region(
            client, 'default-series', 'bionic', cloud=cloud, region=region)
    if other_region is not None:
        log.info('Checking other region model-defaults.')
        assess_model_defaults_region(
            client, 'default-series', 'bionic', cloud=cloud,
            region=other_region)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description='Assess the model-defaults command.')
    add_basic_testing_arguments(parser)
    parser.add_argument('--other-region',
                        help='Set the model default for a different region.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    if (args.other_region is not None and
            args.other_region == bs_manager.client.env.get_region()):
        raise ValueError('Other region is a repeat of region.')
    with bs_manager.booted_context(args.upload_tools):
        assess_model_defaults(bs_manager.client, args.other_region)
    return 0


if __name__ == '__main__':
    sys.exit(main())
