"""
Framework for perfscale testing incl. timing and controller system metric
collection.
"""

from __future__ import print_function

from collections import (
    OrderedDict,
    defaultdict
)
from datetime import datetime
import logging
import os
import re
import subprocess

try:
    import rrdtool
except ImportError:
    # rddtool requires the cairo/pango libs that are difficult to install
    # on non-linux.
    rrdtool = object()
from jinja2 import Template
import json

from logbreakdown import (
    _render_ds_string,
    breakdown_log_by_timeframes,
)
import perf_graphing


__metaclass__ = type


class TimingData:

    strf_format = '%F %H:%M:%S'

    # Log breakdown uses the start/end too. Perhaps have a property for string
    # rep and a ds for the datetime.
    def __init__(self, start, end):
        """Time difference details for an event.

        :param start: datetime.datetime object representing the start of the
          event.
        :param end: datetime.datetime object representing the end of the event.
        """
        self.start = start.strftime(self.strf_format)
        self.end = end.strftime(self.strf_format)
        self.seconds = int((end - start).total_seconds())


class DeployDetails:
    """Details regarding a perfscale testrun.

    :param name: String naming deploy details
    :param applications: A dict containg a {name -> single detail} mapping. For
    instance: {application name -> unit count}
    or {'version' -> juju version string}
    :param timings: A TimingData object representing the test runs start/end
    details
    """
    def __init__(self, name, applications, timings):
        self.name = name
        self.applications = applications
        self.timings = timings


class PerfTestDataJsonSerialisation(json.JSONEncoder):
    def default(self, obj):
        if isinstance(obj, TimingData):
            return obj.__dict__
        if isinstance(obj, DeployDetails):
            return obj.__dict__
        return super(PerfTestDataJsonSerialisation, self).default(obj)


SETUP_SCRIPT_PATH = 'perf_static/setup-perf-monitoring.sh'
COLLECTD_CONFIG_PATH = 'perf_static/collectd.conf'


log = logging.getLogger("run_perfscale_test")


def run_perfscale_test(target_test, bs_manager, args):
    """Run a perfscale test collect the data and generate a report.

    Run the callable `target_test` and collect timing data and system metrics
    for the controller during the test run.

    :param target_test: A callable that takes 2 arguments:
        - EnvJujuClient client object  (bootstrapped)
        - argparse args object
      This callable must return a `DeployDetails` object.

    :param bs_manager: `BootstrapManager` object.
    :param args: `argparse` object to pass to `target_test` when run.
    """
    # XXX
    # Find the actual cause for this!! (Something to do with the template and
    # the logs.)
    import sys
    reload(sys)  # NOQA
    sys.setdefaultencoding('utf-8')
    # XXX

    bs_start = datetime.utcnow()
    with bs_manager.booted_context(args.upload_tools):
        client = bs_manager.client
        admin_client = client.get_controller_client()
        admin_client.wait_for_started()
        bs_end = datetime.utcnow()
        try:
            apply_any_workarounds(client)
            bootstrap_timing = TimingData(bs_start, bs_end)

            setup_system_monitoring(admin_client)

            deploy_details = target_test(client, args)
        finally:
            results_dir = dump_performance_metrics_logs(
                bs_manager.log_dir, admin_client)
            cleanup_start = datetime.utcnow()
    # Cleanup happens when we move out of context
    cleanup_end = datetime.utcnow()
    cleanup_timing = TimingData(cleanup_start, cleanup_end)
    deployments = dict(
        bootstrap=bootstrap_timing,
        deploys=[deploy_details],
        cleanup=cleanup_timing,
    )

    controller_log_file = os.path.join(
        bs_manager.log_dir,
        'controller',
        'machine-0',
        'machine-0.log.gz')

    generate_reports(controller_log_file, results_dir, deployments)


def dump_performance_metrics_logs(log_dir, admin_client):
    results_dir = os.path.join(
        os.path.abspath(log_dir), 'performance_results/')
    os.makedirs(results_dir)
    admin_client.juju(
        'scp',
        ('--', '-r', '0:/var/lib/collectd/rrd/localhost/*',
         results_dir)
    )
    try:
        admin_client.juju(
            'scp', ('0:/tmp/mongodb-stats.log', results_dir)
        )
    except subprocess.CalledProcessError as e:
        log.error('Failed to copy mongodb stats: {}'.format(e))
    return results_dir


def apply_any_workarounds(client):
    # Work around mysql charm wanting 80% memory.
    if client.env.get_cloud() == 'lxd':
        constraint_cmd = [
            'lxc',
            'profile',
            'set',
            'juju-{}'.format(client.env.environment),
            'limits.memory',
            '2GB'
        ]
        subprocess.check_output(constraint_cmd)


def generate_reports(controller_log, results_dir, deployments):
    """Generate reports and graphs from run results."""
    cpu_image = generate_cpu_graph_image(results_dir)
    memory_image = generate_memory_graph_image(results_dir)
    network_image = generate_network_graph_image(results_dir)

    destination_dir = os.path.join(results_dir, 'mongodb')
    os.mkdir(destination_dir)
    try:
        perf_graphing.create_mongodb_rrd_files(results_dir, destination_dir)
    except perf_graphing.SourceFileNotFound:
        log.error(
            'Failed to create the MongoDB RRD file. Source file not found.'
        )

        # Sometimes mongostats fails to startup and start logging. Unsure yet
        # why this is. For now generate the report without the mongodb details,
        # the rest of the report is still useful.
        mongo_query_image = None
        mongo_memory_image = None
    else:
        mongo_query_image = generate_mongo_query_graph_image(results_dir)
        mongo_memory_image = generate_mongo_memory_graph_image(results_dir)

    log_message_chunks = get_log_message_in_timed_chunks(
        controller_log, deployments)

    details = dict(
        cpu_graph=cpu_image,
        memory_graph=memory_image,
        network_graph=network_image,
        mongo_graph=mongo_query_image,
        mongo_memory_graph=mongo_memory_image,
        deployments=deployments,
        log_message_chunks=log_message_chunks
    )

    json_dump_path = os.path.join(results_dir, 'report-data.json')
    with open(json_dump_path, 'wt') as f:
        json.dump(details, f, cls=PerfTestDataJsonSerialisation)

    create_html_report(results_dir, details)


def get_log_message_in_timed_chunks(log_file, deployments):
    """Breakdown log into timechunks based on event timeranges in 'deployments'

    """

    bootstrap = deployments.pop('bootstrap')
    cleanup = deployments.pop('cleanup')
    deployments = deployments.pop('deploys')

    return breakdown_log_by_events_timeframe(
        log_file, bootstrap, cleanup, deployments)


def breakdown_log_by_events_timeframe(log, bootstrap, cleanup, deployments):
    """Breakdown a log file into event chunks.

    Given a log file and time details for events (i.e. bootstrap, cleanup and
    deployments) return a datastructure containing the log contents broken up
    into time chunks relevant for those events.

    :param log: Log file path from which to breakdown data.
    :param bootstrap: TimingData object representing bootstrap timings.
    :param cleanup: TimingData object representing clean timings.
    :param deployments: List of DeployDetails representing each deploy made.
    :returns: OrderedDict of dictionaries, with a structure like:
       {'date range':
            { name, display name, logs -> [{'time frame', display, logs}]}
    """
    name_lookup = _get_log_name_lookup_table(bootstrap, cleanup, deployments)

    deploy_timings = [d.timings for d in deployments]
    raw_details = _get_chunked_log(log, bootstrap, cleanup, deploy_timings)

    # Outer-layer (i.e. event)
    event_details = defaultdict(defaultdict)
    for event_range in raw_details.keys():
        display_name = _display_safe_daterange(event_range)
        event_details[event_range]['name'] = name_lookup[event_range]
        event_details[event_range]['logs'] = []
        event_details[event_range]['event_range_display'] = display_name

        # sort here so that the log list is in order.
        for log_range in raw_details[event_range].keys():
            timeframe = log_range
            display_timeframe = _display_safe_timerange(log_range)
            message = '<br/>'.join(raw_details[event_range][log_range])
            event_details[event_range]['logs'].append(
                dict(
                    timeframe=timeframe,
                    display_timeframe=display_timeframe,
                    message=message))

    return OrderedDict(sorted(event_details.items()))


def _display_safe_daterange(datestamp):
    """Return a datestamp string that can be used as an html class/id."""
    return re.sub('[:\ ]', '', datestamp)


def _display_safe_timerange(timerange):
    """Return a timerange string that can be used as an html class/id."""
    return re.sub('[:\(\)\ ]', '', timerange)


def _get_chunked_log(log_file, bootstrap, cleanup, deployments):
    all_event_timings = [bootstrap] + deployments + [cleanup]
    return breakdown_log_by_timeframes(log_file, all_event_timings)


def _get_log_name_lookup_table(bootstrap, cleanup, deployments):
    """Given event details construct a lookup table to give them names.

    :return: dict containing { daterange -> event name } look up.
    """
    bs_name = _render_ds_string(bootstrap.start, bootstrap.end)
    cleanup_name = _render_ds_string(cleanup.start, cleanup.end)

    name_lookup = {
        bs_name: 'Bootstrap',
        cleanup_name: 'Kill-Controller',
    }
    for dep in deployments:
        name_range = _render_ds_string(
            dep.timings.start, dep.timings.end)
        name_lookup[name_range] = dep.name

    return name_lookup


def create_html_report(results_dir, details):
    template_path = os.path.join(
        os.path.dirname(__file__), 'perf_report_template.html')
    with open(template_path, 'rt') as f:
        template = Template(f.read())

    results_output = os.path.join(results_dir, 'index.html')
    with open(results_output, 'wt') as f:
        f.write(template.render(details))


def generate_graph_image(base_dir, results_dir, name, generator):
    metric_files_dir = os.path.join(os.path.abspath(base_dir), results_dir)
    output_file = os.path.join(
        os.path.abspath(base_dir), '{}.png'.format(name))
    return create_report_graph(metric_files_dir, output_file, generator)


def create_report_graph(rrd_dir, output_file, generator):
    any_file = os.listdir(rrd_dir)[0]
    start, end = get_duration_points(os.path.join(rrd_dir, any_file))
    generator(start, end, rrd_dir, output_file)
    print('Created: {}'.format(output_file))
    return os.path.basename(output_file)


def generate_cpu_graph_image(results_dir):
    return generate_graph_image(
        results_dir, 'aggregation-cpu-average', 'cpu', perf_graphing.cpu_graph)


def generate_memory_graph_image(results_dir):
    return generate_graph_image(
        results_dir, 'memory', 'memory', perf_graphing.memory_graph)


def generate_network_graph_image(results_dir):
    return generate_graph_image(
        results_dir, 'interface-eth0', 'network', perf_graphing.network_graph)


def generate_mongo_query_graph_image(results_dir):
    return generate_graph_image(
        results_dir, 'mongodb', 'mongodb', perf_graphing.mongodb_graph)


def generate_mongo_memory_graph_image(results_dir):
    return generate_graph_image(
        results_dir,
        'mongodb',
        'mongodb_memory',
        perf_graphing.mongodb_memory_graph)


def get_duration_points(rrd_file):
    start = rrdtool.first(rrd_file)
    end = rrdtool.last(rrd_file)

    # Start gives us the start timestamp in the data but it might be null/empty
    # (i.e no data entered for that time.)
    # Find the timestamp in which data was first entered.
    command = [
        'rrdtool', 'fetch', rrd_file, 'AVERAGE',
        '--start', str(start), '--end', str(end)]
    output = subprocess.check_output(command)

    actual_start = find_actual_start(output)

    return actual_start, end


def find_actual_start(fetch_output):
    # Gets a start timestamp this isn't a NaN.
    for line in fetch_output.splitlines():
        try:
            timestamp, value = line.split(':', 1)
            if not value.startswith(' -nan'):
                return timestamp
        except ValueError:
            pass


def setup_system_monitoring(admin_client):
    # Using ssh get into the machine-0 (or all api/state servers)
    # Install the required packages and start up logging of systems collections
    # and mongodb details.

    installer_script_path = _get_static_script_path(SETUP_SCRIPT_PATH)
    collectd_config_path = _get_static_script_path(COLLECTD_CONFIG_PATH)
    installer_script_dest_path = '/tmp/installer.sh'
    runner_script_dest_path = '/tmp/runner.sh'
    collectd_config_dest_file = '/tmp/collectd.config'

    admin_client.juju(
        'scp',
        (collectd_config_path, '0:{}'.format(collectd_config_dest_file)))

    admin_client.juju(
        'scp',
        (installer_script_path, '0:{}'.format(installer_script_dest_path)))
    admin_client.juju('ssh', ('0', 'chmod +x {}'.format(
        installer_script_dest_path)))
    admin_client.juju(
        'ssh',
        ('0', '{installer} {config_file} {output_file}'.format(
            installer=installer_script_dest_path,
            config_file=collectd_config_dest_file,
            output_file=runner_script_dest_path)))

    # Start collection
    # Respawn incase the initial execution fails for whatever reason.
    admin_client.juju('ssh', ('0', '--', 'daemon --respawn {}'.format(
        runner_script_dest_path)))


def _get_static_script_path(script_path):
    full_path = os.path.abspath(__file__)
    current_dir = os.path.dirname(full_path)
    return os.path.join(current_dir, script_path)
