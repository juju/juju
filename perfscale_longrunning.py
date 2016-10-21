from datetime import datetime
import time
import logging

from assess_recovery import deploy_stack
from utility import until_timeout
from generate_perfscale_results import (
    _convert_seconds_to_readable,
    DeployDetails,
    MINUTE,
    TimingData,
)


__metaclass__ = type


total_new_models = 0

log = logging.getLogger("perfscale_longrunning")


class Rest:
    short = MINUTE * 1
    medium = MINUTE * 30
    long = MINUTE * 60
    really_long = MINUTE * 120


def assess_longrun_perf(bs_manager, test_length):
    client = bs_manager.client
    longrun_start = datetime.utcnow()
    run_count = 0
    for _ in until_timeout(test_length):
        applications = ['dummy-sink']
        new_client = action_create(client)
        new_models = action_busy(new_client, applications)
        action_cleanup(new_client, new_models)

        action_rest(Rest.short/2)
        run_count += 1

    longrun_end = datetime.utcnow()
    timing_data = TimingData(longrun_start, longrun_end)
    return DeployDetails(
        'Longrun for {} Hours.'.format(test_length/60/60),
        {'Total action runs': run_count},
        timing_data
    )


def action_create(client, series='trusty'):
    start = datetime.utcnow()
    new_model = client.add_model(client.env.clone('newmodel'))
    deploy_stack(new_model, series)
    end = datetime.utcnow()
    log.info('Create action took: {}'.format(
        _convert_seconds_to_readable(int((end - start).total_seconds()))))
    return new_model


def action_busy(client, applications):
    start = datetime.utcnow()
    client.get_status()

    for app in applications:
        client.juju('add-unit', (app, '-n', '1'))
        client.wait_for_started(timeout=1200)
        client.wait_for_workloads(timeout=1200)

    global total_new_models
    new_models = []
    for i in range(0, 20):
        total_new_models += 1
        new_model_name = 'model{}'.format(total_new_models)
        new_model = client.add_model(client.env.clone(new_model_name))
        new_model.wait_for_started()
        log.info('Added model number {}'.format(total_new_models))
        new_models.append(new_model)

    for _ in until_timeout(MINUTE*2):
        log.info('Checking status ping.')
        client.show_status()
        log.info('Sleeping . . .')
        time.sleep(MINUTE/2)
    end = datetime.utcnow()

    log.info('Create action took: {}'.format(
        _convert_seconds_to_readable(int((end - start).total_seconds()))))

    return new_models


def action_cleanup(client, new_models):
    client.destroy_model()
    for model in new_models:
        model.destroy_model()


def action_rest(rest_length=Rest.short):
    log.info('Resting for {} seconds'.format(rest_length))
    time.sleep(rest_length)
