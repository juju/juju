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
from utility import add_basic_testing_arguments


__metaclass__ = type


MINUTE = 60


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


class PathConfig:
    """Paths for transferring data to a target or running on that target."""
    def __init__(self):
        self.installer_script_path = _get_static_script_path(
            SETUP_SCRIPT_PATH)
        self.collectd_config_path = _get_static_script_path(
            COLLECTD_CONFIG_PATH)
        self.installer_script_dest_path = '/tmp/installer.sh'
        self.runner_script_dest_path = '/tmp/runner.sh'
        self.collectd_config_dest_file = '/tmp/collectd.config'


def _get_static_script_path(script_path):
    full_path = os.path.abspath(__file__)
    current_dir = os.path.dirname(full_path)
    return os.path.join(current_dir, script_path)


SETUP_SCRIPT_PATH = 'perf_static/setup-perf-monitoring.sh'
COLLECTD_CONFIG_PATH = 'perf_static/collectd.conf'


PATHS = PathConfig()


log = logging.getLogger("run_perfscale_test")


def add_basic_perfscale_arguments(parser):
    """Add the basic required args needed for a perfscale test."""
    add_basic_testing_arguments(parser)
    parser.add_argument(
        '--enable-ha',
        help='Enable HA before running perfscale test.',
        action='store_true')


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

            maybe_enable_ha(admin_client, args)

            machine_ids = setup_system_monitoring(admin_client)

            deploy_details = target_test(client, args)
        finally:
            results_dir = dump_performance_metrics_logs(
                bs_manager.log_dir, admin_client, machine_ids)
            cleanup_start = datetime.utcnow()
    # Cleanup happens when we move out of context
    cleanup_end = datetime.utcnow()
    cleanup_timing = TimingData(cleanup_start, cleanup_end)

    total_timing = TimingData(bs_start, cleanup_end)
    output_test_run_length(total_timing.seconds)

    graph_period = _determine_graph_period(total_timing.seconds)
    deployments = dict(
        bootstrap=bootstrap_timing,
        deploys=[deploy_details],
        cleanup=cleanup_timing,
    )

    generate_reports(
        bs_manager.log_dir,
        results_dir,
        deployments,
        machine_ids,
        graph_period)


def output_test_run_length(seconds):
    time_taken = _convert_seconds_to_readable(seconds)

    log.info('Test took: {}'.format(time_taken))


def _convert_seconds_to_readable(seconds):
    """Given a period in seconds break it down into hour, minute & seconds."""
    m, s = divmod(seconds, 60)
    h, m = divmod(m, 60)
    return '{0}h:{1:02d}m:{2:02d}s'.format(h, m, s)


def _determine_graph_period(seconds):
    """Given a tests length determine what graphing period is needed."""
    if seconds >= (MINUTE * 60 * 2):
        return perf_graphing.GraphPeriod.day
    return perf_graphing.GraphPeriod.hours


def dump_performance_metrics_logs(log_dir, admin_client, machine_ids):
    """Pull metric logs and data off every controller machine in action.

    Store the retrieved data in a machine-id named directory underneath the
    genereated (and returned) base directory.

    :return: Path string indicating the base path of data retrieved from the
      controllers.
    """
    base_results_dir = os.path.join(
        os.path.abspath(log_dir), 'performance_results/')

    for machine_id in machine_ids:
        results_dir = os.path.join(
            base_results_dir, 'machine-{}'.format(machine_id))
        os.makedirs(results_dir)

        admin_client.juju(
            'scp',
            ('--proxy', '--',
             '-r',
             '{}:/var/lib/collectd/rrd/localhost/*'.format(machine_id),
             results_dir)
        )
        try:
            admin_client.juju(
                'scp',
                ('--proxy',
                 '{}:/tmp/mongodb-stats.log'.format(machine_id),
                 results_dir))
        except subprocess.CalledProcessError as e:
            log.error('Failed to copy mongodb stats for machine {}: {}'.format(
                machine_id, e))
    return base_results_dir


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


def maybe_enable_ha(admin_client, args):
    if args.enable_ha:
        log.info('Enabling HA.')
        admin_client.enable_ha()
        admin_client.wait_for_ha()


def generate_reports(
        log_dir, results_dir, deployments, machine_ids, graph_period):
    """Generate graph image from run results for each controller in action."""

    for m_id in machine_ids:
        machine_results_dir = os.path.join(
            results_dir, 'machine-{}'.format(m_id))
        _create_graph_images_for_machine(
            m_id, machine_results_dir, graph_period)

    # This will take care of making sure machine-0 is named log_message_chunks.
    log_chunks = _get_controller_log_message_chunks(
        log_dir, machine_ids, deployments, graph_period)

    details = dict(
        deployments=deployments,
        **log_chunks
    )

    json_dump_path = os.path.join(results_dir, 'report-data.json')
    with open(json_dump_path, 'wt') as f:
        json.dump(details, f, cls=PerfTestDataJsonSerialisation)


def _get_controller_log_message_chunks(
        log_dir, machine_ids, deployments, graph_period):
    """Produce 'chunked' logs for the provided controller ids.

    :return: dict containing chunked logs for a controller machine. The key
      indicates which controller it's from.
      i.e. log_message_chunks_2 is from machine-2. (Note. naming is due to
      backwards compatibility for existing data collection.)
    """
    log_chunks = dict()
    for m_id in machine_ids:
        machine_log_file = os.path.join(
            log_dir,
            'controller',
            'machine-{}'.format(m_id),
            'machine-{}.log.gz'.format(m_id))

        log_name = 'log_message_chunks_{}'.format(m_id)
        # Skip doing the logs for long runs (the logs can get huge).
        if graph_period == perf_graphing.GraphPeriod.hours:
            log_chunks[log_name] = breakdown_log_by_events_timeframe(
                machine_log_file,
                deployments['bootstrap'],
                deployments['cleanup'],
                deployments['deploys'])
        else:
            log_chunks[log_name] = defaultdict(defaultdict)
    # Keep backwards compatible data naming (for before collecting HA results).
    log_chunks['log_message_chunks'] = log_chunks.pop('log_message_chunks_0')
    return log_chunks


def _create_graph_images_for_machine(machine_id, results_dir, graph_period):
    """Create graph images from the data from `machine_id`s details."""
    generate_cpu_graph_image(results_dir, graph_period)
    generate_memory_graph_image(results_dir, graph_period)
    generate_network_graph_image(results_dir, graph_period)

    destination_dir = os.path.join(results_dir, 'mongodb')
    os.mkdir(destination_dir)
    try:
        perf_graphing.create_mongodb_rrd_files(results_dir, destination_dir)
    except (perf_graphing.SourceFileNotFound, perf_graphing.NoDataPresent):
        # Sometimes mongostats fails to startup and start logging. Unsure yet
        # why this is. For now generate the report without the mongodb details,
        # the rest of the report is still useful.
        log.error(
            'Failed to create the MongoDB RRD file. '
            'Source file empty or not found.'
        )
    else:
        generate_mongo_query_graph_image(results_dir, graph_period)
        generate_mongo_memory_graph_image(results_dir, graph_period)


def breakdown_log_by_events_timeframe(log, bootstrap, cleanup, deployments):
    """Breakdown a log file into event chunks.

    Given a log file and time details for events (i.e. bootstrap, cleanup and
    deployments) return a datastructure containing the log contents broken up
    into time chunks relevant for those events.

    :param log: Log file path from which to breakdown data.
    :param bootstrap: TimingData object representing bootstrap timings.
    :param cleanup: TimingData object representing clean timings.
    :param deployments: List of DeployDetails representing each deploy made.
    :return: OrderedDict of dictionaries, with a structure like:
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


def generate_graph_image(base_dir, results_dir, name, generator, graph_period):
    """Generate graph image files.

    The images will have the machine id encoded within the names.
    i.e. machine-0-cpu.png
    """
    metric_files_dir = os.path.join(os.path.abspath(base_dir), results_dir)
    output_file = _image_name(base_dir, name)
    try:
        return create_report_graph(
            metric_files_dir, output_file, generator, graph_period)
    except perf_graphing.SourceFileNotFound:
        # It's possible that a HA controller isn't around long enough to
        # actually gather some data from resulting in a lack of rrd file for
        # that metric.
        log.warning('Failed to generate {}.'.format(output_file))


def _image_name(base_dir, name):
    # Encode the machine id into the image name. The machine id is part of the
    # directory structure.
    basename = os.path.basename(os.path.normpath(base_dir))
    return os.path.join(base_dir, '{}-{}.png'.format(basename, name))


def create_report_graph(rrd_dir, output_file, generator, graph_period):
    any_file = os.listdir(rrd_dir)[0]
    start, end = get_duration_points(
        os.path.join(rrd_dir, any_file), graph_period)
    generator(start, end, rrd_dir, output_file)
    print('Created: {}'.format(output_file))
    return os.path.basename(output_file)


def generate_cpu_graph_image(results_dir, graph_period):
    return generate_graph_image(
        results_dir,
        'aggregation-cpu-max',
        'cpu',
        perf_graphing.cpu_graph,
        graph_period)


def generate_memory_graph_image(results_dir, graph_period):
    return generate_graph_image(
        results_dir,
        'memory',
        'memory',
        perf_graphing.memory_graph,
        graph_period)


def generate_network_graph_image(results_dir, graph_period):
    return generate_graph_image(
        results_dir,
        'interface-eth0',
        'network',
        perf_graphing.network_graph,
        graph_period)


def generate_mongo_query_graph_image(results_dir, graph_period):
    return generate_graph_image(
        results_dir,
        'mongodb',
        'mongodb',
        perf_graphing.mongodb_graph,
        graph_period)


def generate_mongo_memory_graph_image(results_dir, graph_period):
    return generate_graph_image(
        results_dir,
        'mongodb',
        'mongodb_memory',
        perf_graphing.mongodb_memory_graph,
        graph_period
    )


def get_duration_points(rrd_file, graph_period):
    start = rrdtool.first(rrd_file, '--rraindex', graph_period)
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


def get_controller_machine_ids(admin_client):
    """Returns list of machine ids for all active controller machines."""
    machines = admin_client.get_controller_members()
    return [m.machine_id for m in machines]


def setup_system_monitoring(admin_client):
    """Setup metrics collections for all controller machines in action."""
    # For all contrller machines we need to get what machines they are and
    # install on them.

    controller_machine_ids = get_controller_machine_ids(admin_client)

    for machine_id in controller_machine_ids:
        _setup_system_monitoring(admin_client, machine_id)

    # Start logging separate to setup so things start almost at the same time
    # (not waiting around for other machines to setup.)
    for machine_id in controller_machine_ids:
        _enable_monitoring(admin_client, machine_id)

    return controller_machine_ids


def _setup_system_monitoring(admin_client, machine_id):
    """Install required metrics monitoring software on supplied machine id.

    Using ssh & scp get into the controller machines install the required
    packages and start up logging of systems collections and mongodb details.
    """
    admin_client.juju(
        'scp',
        ('--proxy', PATHS.collectd_config_path, '{}:{}'.format(
            machine_id, PATHS.collectd_config_dest_file)))

    admin_client.juju(
        'scp',
        ('--proxy', PATHS.installer_script_path, '{}:{}'.format(
            machine_id, PATHS.installer_script_dest_path)))
    admin_client.juju('ssh', ('--proxy', machine_id, 'chmod +x {}'.format(
        PATHS.installer_script_dest_path)))


def _enable_monitoring(admin_client, machine_id):
    # Start collection
    # Respawn incase the initial execution fails for whatever reason.
    admin_client.juju(
        'ssh',
        ('--proxy',
         machine_id,
         '{installer} {config_file} {output_file}'.format(
             installer=PATHS.installer_script_dest_path,
             config_file=PATHS.collectd_config_dest_file,
             output_file=PATHS.runner_script_dest_path)))

    admin_client.juju(
        'ssh',
        ('--proxy', machine_id, '--', 'daemon --respawn {}'.format(
            PATHS.runner_script_dest_path)))
