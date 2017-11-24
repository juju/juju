#!/usr/bin/env python
"""Functional tests for Cross Model Relation (CMR) functionality.

This test exercises the CMR Juju functionality which allows applications to
communicate between different models (including across controllers/clouds).

The outline of this feature can be found here[1].

This test will exercise the following aspects:
  - Ensure a user is able to create an offer of an applications' endpoint
    including:
      - A user is able to consume and relate to the offer
      - The offer appears in the list-offers output
      - The user is able to name the offer
      - The user is able to remove the offer

The above feature tests will be run on:
  - A single controller environment
  - Multiple controllers where each controller is in a different cloud.


[1] https://docs.google.com/document/d/1IBTrqQSP3nrx5mTd_1vtUJ5YF28u9KJNTldUmUqrkJM/  # NOQA
"""

from __future__ import print_function

import argparse
import logging
import sys
import yaml
from contextlib import contextmanager
from subprocess import CalledProcessError
from textwrap import dedent

from assess_recovery import check_token
from deploy_stack import (
    BootstrapManager,
    get_random_string,
    )
from jujucharm import local_charm_path
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
    )


__metaclass__ = type


log = logging.getLogger("assess_cross_model_relations")


def assess_cross_model_relations_single_controller(args):
    """Assess that offers can be consumed in models on the same controller."""
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        first_model = bs_manager.client
        with temporary_model(first_model, 'second-model') as second_model:
                ensure_cmr_offer_management(first_model)
                ensure_cmr_offer_consumption_and_relation(
                    first_model, second_model)


def assess_cross_model_relations_multiple_controllers(args):
    """Offers must be able to consume models on different controllers."""
    second_args = extract_second_provider_details(args)
    second_bs_manager = BootstrapManager.from_args(second_args)

    first_bs_manager = BootstrapManager.from_args(args)
    with first_bs_manager.booted_context(args.upload_tools):
        first_model = first_bs_manager.client
        # Need to share juju_data/home
        second_bs_manager.client.env.juju_home = first_model.env.juju_home
        with second_bs_manager.existing_booted_context(
                second_args.upload_tools):
            second_model = second_bs_manager.client
            ensure_cmr_offer_consumption_and_relation(
                first_model, second_model)


def ensure_cmr_offer_management(client):
    """Ensure creation, listing and deletion of offers work.

    Deploy dummy-source application onto `client` and offer it's endpoint.
    Ensure that:
      - The offer attempt is successful
      - The offer is shown in 'list-offers'
      - The offer can be deleted (and no longer appear in 'list-offers')

    :param client:   ModelClient used to create a new model and attempt 'offer'
      commands on
    """
    with temporary_model(client, 'offer-management') as management_model:
        app_name = 'dummy-source'

        deploy_local_charm(management_model, app_name)

        offer_url = assert_offer_is_listed(
            management_model, app_name, offer_name='kitchen-sink')
        assert_offer_can_be_deleted(management_model, offer_url)

        offer_url = assert_offer_is_listed(management_model, app_name)
        assert_offer_can_be_deleted(management_model, offer_url)


def assert_offer_is_listed(client, app_name, offer_name=None):
    """Assert that an offered endpoint is listed.

    :param client: ModelClient for model to use.
    :param app_name: Name of the deployed application to make an offer for.
    :param offer_name: If not None is used to name the endpoint offer.
    :return: String URL of the resulting offered endpoint.
    """
    log.info('Assessing {} offers.'.format(
        'named' if offer_name else 'unnamed'))

    expected_url, offer_key = offer_endpoint(client, app_name, offer_name)
    offer_output = yaml.safe_load(
        client.get_juju_output('offers', '--format', 'yaml'))

    fully_qualified_offer = '{controller}:{offer_url}'.format(
        controller=client.env.controller.name,
        offer_url=offer_output[offer_key]['offer-url'])
    try:
        if fully_qualified_offer != expected_url:
            raise JujuAssertionError(
                'Offer URL mismatch.\n{actual} != {expected}'.format(
                    actual=offer_output[offer_key]['offer-url'],
                    expected=expected_url))
    except KeyError:
        raise JujuAssertionError('No offer URL found in offers output.')

    log.info('PASS: Assert offer is listed.')
    return expected_url


def assert_offer_can_be_deleted(client, offer_url):
    """Assert that an offer can be successfully deleted."""
    client.juju('remove-offer', (offer_url), include_e=False)
    offer_output = yaml.safe_load(
        client.get_juju_output('offers', '--format', 'yaml'))

    if offer_output != {}:
        raise JujuAssertionError(
            'Failed to remove offer "{}"'.format(offer_url))
    log.info('PASS: Assert offer is removed.')


def ensure_cmr_offer_consumption_and_relation(first_client, second_client):
    """Ensure offers can be consumed by another model.

    :param first_client: ModelClient model that will be the source of the
      offer.
    :param second_client: ModelClient model that will consume the offered
      application endpoint.
    """
    token = get_random_string()
    deploy_local_charm(first_client, 'dummy-source')
    first_client.set_config('dummy-source', {'token': token})
    first_client.wait_for_workloads()

    deploy_local_charm(second_client, 'dummy-sink')

    offer_url, offer_name = offer_endpoint(first_client, 'dummy-source')

    second_client.juju('relate', ('dummy-sink', offer_url))
    second_client.wait_for_workloads()
    check_token(second_client, token)

    status_saas_check = second_client.get_status()
    status_saas_url = status_saas_check.status[
        'application-endpoints'][offer_name]['url']
    if status_saas_url != offer_url:
        raise JujuAssertionError(
            'Consuming models status does not state status of the consumed'
            ' offer.')


def offer_endpoint(client, app_name, offer_name=None):
    """Create an endpoint offer for `app_name` with optional name.

    :param client: ModelClient of model to operate on.
    :param app_name: Deployed application name to create offer for.
    :param offer_name: If not None create the offer with this name.
    :return: Tuple of the resulting offer url (including controller) and the
      offer name (default or named).
    """
    model_name = client.env.environment
    offer_endpoint = '{model}.{app}:{relation}'.format(
        model=model_name,
        app=app_name,
        relation='sink')
    offer_args = [offer_endpoint, '-c', client.env.controller.name]
    if offer_name:
        offer_args.append(offer_name)
    client.juju('offer', tuple(offer_args), include_e=False)

    offer_name = offer_name if offer_name else app_name
    offer_url = '{controller}:{user}/{model}.{offer}'.format(
        controller=client.env.controller.name,
        user=client.env.user_name,
        model=client.env.environment,
        offer=offer_name)
    return offer_url, offer_name


def deploy_local_charm(client, app_name):
    charm_path = local_charm_path(
        charm=app_name, juju_ver=client.version)
    client.deploy(charm_path)
    client.wait_for_started()


@contextmanager
def temporary_model(client, model_name):
    """Create a new model that is cleaned up once it's done with."""
    try:
        new_client = client.add_model(model_name)
        yield new_client
    finally:
        try:
            log.info('Destroying temp model "{}"'.format(model_name))
            new_client.destroy_model()
        except CalledProcessError:
            log.info('Failed to cleanup model.')


def extract_second_provider_details(args):
    """Create a Namespace suitable for use with BootstrapManager.from_args.

    Using the 'secondary' environment details returns a argparse.Namespace
    object that can be used with BootstrapManager.from_args to get a
    bootstrap-able BootstrapManager.
    """
    new_args = vars(args).copy()
    new_args['env'] = new_args['secondary_env']
    new_args['region'] = new_args.get('secondary-region')
    new_args['temp_env_name'] = '{}-secondary'.format(
        new_args['temp_env_name'])
    return argparse.Namespace(**new_args)


def parse_args(argv):
    parser = argparse.ArgumentParser(
        description="Cross Model Relations functional test.")
    parser.add_argument(
        '--secondary-env',
        help=dedent("""\
            The second provider to use for the test.
            If set the test will assess CMR functionality between the provider
            set in `primary-env` and this env (`secondary-env`).
            """))
    parser.add_argument(
        '--secondary-region',
        help='Override the default region for the secondary environment.')
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    assess_cross_model_relations_single_controller(args)

    if args.secondary_env:
        log.info('Assessing multiple controllers.')
        assess_cross_model_relations_multiple_controllers(args)

    return 0


if __name__ == '__main__':
    sys.exit(main())
