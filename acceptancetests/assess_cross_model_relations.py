#!/usr/bin/env python
"""Functional tests for Cross Model Relation (CMR) functionality.

This test exercises the CMR Juju functionality which allows applications to
communicate between different models (including across controllers/clouds).

The outline of this feature can be found here[1].

This test will exercise the following aspects:
  - Ensure a user is able to create an offer of an applications' endpoint
    including:
      - A user is able to consume and relate to the offer
      - Workload data successfully provided
      - The offer appears in the list-offers output
      - The user is able to name the offer
      - The user is able to remove the offer
  - Ensure an admin can grant a user access to an offer
      - The consuming user finds the offer via 'find-offer'

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
from textwrap import dedent

from deploy_stack import (
    BootstrapManager,
    )
from jujupy.client import (
    Controller,
    register_user_interactively,
    )
from jujupy.models import (
    temporary_model
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
        offer_model = bs_manager.client
        with temporary_model(offer_model, 'consume-model') as consume_model:
            ensure_cmr_offer_management(offer_model)
            ensure_cmr_offer_consumption_and_relation(offer_model, consume_model)


def assess_cross_model_relations_multiple_controllers(args):
    """Offers must be able to consume models on different controllers."""
    consume_bs_args = extract_second_provider_details(args)
    consume_bs_manager = BootstrapManager.from_args(consume_bs_args)

    offer_bs_manager = BootstrapManager.from_args(args)
    with offer_bs_manager.booted_context(args.upload_tools):
        offer_model = offer_bs_manager.client
        with consume_bs_manager.booted_context(consume_bs_args.upload_tools):
            consume_model = consume_bs_manager.client
            ensure_user_can_consume_offer(offer_model, consume_model)
            log.info("Finished CMR multiple controllers test.")


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
        log.info('Asserting CMR offer management.')
        app_name = 'dummy-source'

        deploy_local_charm(management_model, app_name)

        offer_url = assert_offer_is_listed(
            management_model, app_name, offer_name='kitchen-sink')
        assert_offer_can_be_deleted(management_model, offer_url)

        offer_url = assert_offer_is_listed(management_model, app_name)
        assert_offer_can_be_deleted(management_model, offer_url)
        log.info('PASS: CMR offer management.')


def ensure_cmr_offer_consumption_and_relation(offer_client, consume_client):
    """Ensure offers can be consumed by another model.

    :param offer_client: ModelClient model that will be the source of the
      offer.
    :param consume_client: ModelClient model that will consume the offered
      application endpoint.
    """
    with temporary_model(offer_client, 'relation-source') as source_client:
        with temporary_model(consume_client, 'relation-sink') as sink_client:
            offer_url, offer_name = deploy_and_offer_db_app(source_client)
            assert_relating_to_offer_succeeds(sink_client, offer_url)
            assert_saas_url_is_correct(sink_client, offer_name, offer_url)


def ensure_user_can_consume_offer(offer_client, consume_client):
    """Ensure admin is able to grant a user access to an offer.

    Almost the same as `ensure_cmr_offer_consumption_and_relation` except in
    this a user is created on all controllers (might be a single controller
    or 2) with the permissions of 'login' for the source controller and 'write'
    for the model into which the user will deploy an application to consume the
    offer.

    :param offer_client: ModelClient model that will be the source of the
      offer.
    :param consume_client: ModelClient model that will consume the offered
      application endpoint.
    """
    with temporary_model(offer_client, 'relation-source') as source_client:
        with temporary_model(consume_client, 'relation-sink') as sink_client:
            offer_url, offer_name = deploy_and_offer_db_app(source_client)
            log.info('Asserting offer {} can be consumed.'.format(offer_url))

            username = 'theundertaker'
            token = source_client.add_user_perms(username)
            user_sink_client = register_user_on_controller(
                sink_client, username, token)

            source_client.controller_juju(
                'grant',
                (username, 'consume', offer_url))

            offers_found = yaml.safe_load(
                user_sink_client.get_juju_output(
                    'find-offers',
                    '--interface', 'mysql',
                    '--format', 'yaml',
                    include_e=False))
            # There must only be one offer
            user_offer_url = list(offers_found.keys())[0]

            assert_relating_to_offer_succeeds(sink_client, user_offer_url)
            assert_saas_url_is_correct(
                sink_client, offer_name, user_offer_url)


def assert_offer_is_listed(client, app_name, offer_name=None):
    """Assert that an offered endpoint is listed.

    :param client: ModelClient for model to use.
    :param app_name: Name of the deployed application to make an offer for.
    :param offer_name: If not None is used to name the endpoint offer.
    :return: String URL of the resulting offered endpoint.
    """
    log.info('Asserting listing {} offers.'.format(
        'named' if offer_name else 'unnamed'))

    expected_url, offer_key = offer_endpoint(
        client, app_name, 'sink', offer_name=offer_name)
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
    log.info('Asserting offer can be deleted.')
    client.juju('remove-offer', (offer_url), include_e=False)
    offer_output = yaml.safe_load(
        client.get_juju_output('offers', '--format', 'yaml'))

    if offer_output != {}:
        raise JujuAssertionError(
            'Failed to remove offer "{}"'.format(offer_url))
    log.info('PASS: Assert offer is removed.')


def assert_relating_to_offer_succeeds(client, offer_url):
    """Deploy mediawiki on client and relate to provided `offer_url`.

    Raises an exception if the workload status does not move to 'active' within
    the default timeout (600 seconds).
    """
    log.info('Asserting relating to offer.')
    client.deploy('cs:mediawiki')
    # No need to check workloads until the relation is set.
    client.wait_for_started()
    # mediawiki workload is blocked ('Database needed') until a db
    # relation is successfully made.
    client.juju('relate', ('mediawiki:db', offer_url))
    client.wait_for_workloads()
    log.info('PASS: Relating mediawiki to mysql offer.')


def assert_saas_url_is_correct(client, offer_name, offer_url):
    """Offer url of Saas status field must match the expected `offer_url`."""
    log.info('Asserting SAAS URL is correct.')
    status_saas_check = client.get_status()
    status_saas_url = status_saas_check.status[
        'application-endpoints'][offer_name]['url']
    if status_saas_url != offer_url:
        raise JujuAssertionError(
            'Consuming models status does not state status of the'
            'consumed offer.')
    log.info('PASS: SAAS URL is correct.')

def deploy_and_offer_db_app(client):
    """Deploy mysql application and offer it's db endpoint.

    :return: tuple of (resulting offer url, offer name)
    """
    client.deploy('cs:mysql')
    client.wait_for_started()
    client.wait_for_workloads()
    return offer_endpoint(client, 'mysql', 'db', offer_name='offered-mysql')


def offer_endpoint(client, app_name, relation_name, offer_name=None):
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
        relation=relation_name)
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


def register_user_on_controller(client, username, token):
    """Register user with `token` on `client`s controller.

    :return: ModelClient object for the registered user.
    """
    controller_name = 'cmr_test'
    user_client = client.clone(env=client.env.clone())
    user_client.env.user_name = username
    user_client.env.controller = Controller(controller_name)
    register_user_interactively(user_client, token, controller_name)
    return user_client


def deploy_local_charm(client, app_name):
    charm_path = local_charm_path(
        charm=app_name, juju_ver=client.version)
    client.deploy(charm_path)
    client.wait_for_started()


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
