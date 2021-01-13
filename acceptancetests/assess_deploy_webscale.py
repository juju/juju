#!/usr/bin/env python3
""" Test webscale deployment

    1. deploying kubernetes core and asserting it is `healthy`
    2. inspect the logs to parse timings from trace logs
    3. send timings to the reporting client
       a. include charm revisions in the tags
"""

from __future__ import print_function

import argparse
import logging
import sys
import os
import re
import statistics
import time
import shutil

from urllib.parse import urlparse
from urllib.request import urlretrieve

from deploy_stack import (
    BootstrapManager,
    deploy_iaas_stack,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    JujuAssertionError,
)

from jujucharm import (
    local_charm_path,
)
from reporting import (
    get_reporting_client,
)

__metaclass__ = type

log = logging.getLogger("assess_deploy_webscale")


def deploy_bundle(client, charm_bundle, stack_type):
    """Deploy the given charm bundle

    :param client: Jujupy ModelClient object
    :param charm_bundle: Optional charm bundle string
    """
    bundle = None
    if not charm_bundle:
        # Depending on the stack type, use a different bundle definition for
        # one that's not already included.
        if stack_type == "iaas":
            default_charm = "webscale-lxd.yaml"
        else:
            raise JujuAssertionError(
                'invalid stack type {}'.format(stack_type))

        bundle = local_charm_path(
            charm=default_charm,
            repository=os.environ['JUJU_REPOSITORY'],
            juju_ver=client.version,
        )
    else:
        bundle = charm_bundle

    stack_client = get_stack_client(
        stack_type,
        path=bundle,
        client=client,
        charm=(not not charm_bundle),
        timeout=43200,
    )

    if not stack_client.is_cluster_healthy:
        raise JujuAssertionError('cluster is not healthy')


def extract_module_logs(client, module):
    """Extract the logs from destination module.

    :param module: string containing the information to extract from the
    destination module.
    """
    deploy_logs = client.get_juju_output(
        'debug-log', '-m', 'controller',
        '--no-tail', '--replay', '-l', 'TRACE',
        '--include-module', module,
    )
    return deploy_logs.decode('utf-8')


def extract_txn_metrics(logs, module):
    """Extract the transaction timings and retry counts from the deploy logs.

    It's expected that the timings are in seconds to 3 decimal places
    ("0.042s")

    :param logs: string containing all the logs from the module
    :param module: string containing the destination module.
    """
    exp = "{} ran transaction in " \
        r"(?P<seconds>\d+\.\d+)s \(retries: (?P<retries>\d+)\)"
    regex = re.compile(exp.format(module), re.IGNORECASE)
    timings = []
    retries = []
    for match in regex.finditer(logs):
        timings.append(float(match.group("seconds")))
        retries.append(float(match.group("retries")))

    return {
        'timings': timings,
        'retries': retries,
    }


def calc_stats(prefix, values, include_count=False, test_duration=0):
    """ Calculate statistics for a list of float values and return them as an
        object where the keys are prefixed using the provided prefix.
    """
    stats = {
        prefix+'min': min(values),
        prefix+'max': max(values),
        prefix+'total': sum(values),
        prefix+'mean': statistics.mean(values),
        prefix+'median': statistics.median(values),
        prefix+'stdev': statistics.stdev(values),
    }

    if include_count:
        stats[prefix+'count'] = len(values)

    if test_duration != 0:
        stats[prefix+'rate'] = float(len(values)) / float(test_duration)

    return stats


def merge_dicts(*args):
    out = {}
    for d in args:
        out.update(d)

    return out


def construct_metrics(txn_metrics, test_duration):
    """Make metrics creates a dictionary of items to pass to the
       reporting client.
    """

    return merge_dicts(
        calc_stats('txn_time_', txn_metrics['timings'], include_count=True,
                   test_duration=test_duration),
        calc_stats('txn_retries_', txn_metrics['retries']),
        {'test_duration': test_duration},
    )


def extract_charm_urls(client):
    """Extract the bundle with revisions
    """
    status = client.get_status()
    application_info = status.get_applications()
    charms = []
    for charm in application_info.values():
        charms.append(charm["charm"])
    return charms


def extract_mongo_details(client):
    """Extract the mongo version and profile from the controller.
    """

    ctrl_info = client.get_controllers()
    ctrl = ctrl_info.get_controller(client.env.controller.name)
    ctrl_details = ctrl.get_details()

    ctrl_config = client.get_controller_config(client.env.controller.name)

    return ctrl_details.mongo_version, ctrl_config.mongo_memory_profile


def get_stack_client(stack_type, path, client, timeout=3600, charm=False):
    """Get the stack client dependant on the type of stack we want to deploy on
    """
    if stack_type == "iaas":
        fn = deploy_iaas_stack
    else:
        raise JujuAssertionError('invalid stack type {}'.format(stack_type))
    return fn(path, client, timeout=timeout, charm=charm)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Webscale charm deployment CI test")
    parser.add_argument(
        '--charm-bundle',
        help="Override the charm bundle to deploy",
    )
    parser.add_argument(
        '--logging-module',
        help="Override default module to extract",
        default="juju.state.txn",
    )
    parser.add_argument(
        '--git-sha',
        help="Help the reporting metrics by supplying a git SHA",
        default="",
    )
    parser.add_argument(
        '--reporting-uri',
        help="Reporting uri for sending the metrics to.",
        default="http://root:root@localhost:8086",
    )
    parser.add_argument(
        '--stack-type',
        help="Stack type to use when deploying <iaas>",
        default="iaas",
    )
    parser.add_argument(
        '--juju-version',
        help="Help the reporting metrics by supplying a target juju version",
        default="",
    )
    parser.add_argument(
        '--db-snap-path',
        help="URL to a mongo snap to override the mongo version used for the "
             "test controller; if a URL is specified, it will be downloaded "
             "locally before bootstrapping",
        default="",
    )
    parser.add_argument(
        '--db-asserts-path',
        help="URL to a mongo asserts file to be used in conjunction with a "
             "provided mongo snap to bootstrap the test controller; if a URL "
             "is specified, it will be downloaded locally before "
             "bootstrapping",
        default="",
    )
    parser.add_argument(
        '--mongo-memory-profile',
        help="the name of a mongo profile to use when bootstrapping",
        default="",
    )
    parser.add_argument(
        '--with-mongo-server-side-txns',
        help="set to true to enable server-side mongo transactions (mongo4)",
        default="false",
    )
    add_basic_testing_arguments(parser, existing=False)
    # Override the default logging_config default value set by adding basic
    # testing arguments. This way we can have a default value for all tests,
    # then override it again just for this test.
    parser.set_defaults(
        logging_config="juju.state.txn=TRACE;<root>=INFO;unit=INFO")
    return parser.parse_args(argv)


def mongo_snap_settings(args):
    if not args.db_snap_path and not args.db_asserts_path:
        log.info("Using built-in mongo")
        return "", ""

    # NOTE(achilleasa): ideally we should be using client.enable_feature()
    # but it looks like the current implementation does not pass the
    # enabled features to the backend; the backend however supports
    # fetching the features through the JUJU_DEV_FEATURE_FLAGS envvar.
    log.info("Enabling 'mongodb-snap' feature flag")
    flags = "mongodb-snap"
    if args.with_mongo_server_side_txns == "true":
        log.info("Enabling 'mongodb-sstxn' feature flag")
        flags += ",mongodb-sstxn"

    os.environ["JUJU_DEV_FEATURE_FLAGS"] = flags

    # Fetch snap if using a remote URL. Juju expects the snap file to match
    # "juju-db_\d+.snap" so we should rename it accordingly.
    dst_snap_path = "/tmp/juju-db_6.snap"

    if urlparse(args.db_snap_path).scheme != "":
        log.info("Downloading db snap from {} to {}".format(
            args.db_snap_path, dst_snap_path))
        urlretrieve(args.db_snap_path, dst_snap_path)
    else:
        log.info("Copying local db snap {} to {}".format(
            args.db_snap_path, dst_snap_path))
        shutil.copy2(args.db_snap_path, dst_snap_path)

    # Fetch asserts file if using a remote URL
    dst_asserts_path = "/tmp/juju-db_6.asserts"

    if urlparse(args.db_asserts_path).scheme != "":
        log.info("Downloading db asserts from {} to {}".format(
            args.db_asserts_path, dst_asserts_path))
        urlretrieve(args.db_asserts_path, dst_asserts_path)
    else:
        log.info("Copying local db asserts {} to {}".format(
            args.db_asserts_path, dst_asserts_path))
        shutil.copy2(args.db_asserts_path, dst_asserts_path)

    return dst_snap_path, dst_asserts_path


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    db_snap_path, db_snap_asserts_path = mongo_snap_settings(args)

    begin = time.time()
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools,
                                   db_snap_path=db_snap_path,
                                   db_snap_asserts_path=db_snap_asserts_path,
                                   mongo_memory_profile=args.
                                   mongo_memory_profile):
        client = bs_manager.client
        mongo_version, mongo_profile = extract_mongo_details(client)
        log.info("MongoVersion used for deployment: {} (profile: {})".format(
            mongo_version, mongo_profile))

        deploy_bundle(
                client,
                charm_bundle=args.charm_bundle,
                stack_type=args.stack_type,
        )
        raw_logs = extract_module_logs(client, module=args.logging_module)
        txn_metrics = extract_txn_metrics(raw_logs, module=args.logging_module)

        # Calculate the timings to forward to the datastore
        metrics = construct_metrics(txn_metrics, (time.time() - begin))
        log.info("Metrics for deployment: {}".format(metrics))

        # Extract the charm bundle and revision numbers
        charm_urls = ",".join(extract_charm_urls(client))

        try:
            use_sst = args.with_mongo_server_side_txns == "true"
            rclient = get_reporting_client(args.reporting_uri)
            rclient.report(metrics, tags={
                "git-sha": args.git_sha,
                "charm-bundle": args.charm_bundle,
                "charm-urls": charm_urls,
                "juju-version": args.juju_version,
                "mongo-version": mongo_version,
                "mongo-profile": mongo_profile,
                "mongo-ss-txns": "true" if use_sst else "false",
            })
        except Exception:
            raise JujuAssertionError("Error reporting metrics")
        log.info("Metrics successfully sent to report storage")
    return 0


if __name__ == '__main__':
    sys.exit(main())
