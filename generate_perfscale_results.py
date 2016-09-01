#!/usr/bin/env python
"""Simple performance and scale tests."""

from __future__ import print_function

import argparse
from collections import defaultdict, namedtuple
from datetime import datetime
import logging
import os
import sys
import subprocess
import time

import rrdtool
from jinja2 import Template
from fixtures import EnvironmentVariable

from deploy_stack import (
    BootstrapManager,
)
from logbreakdown import (
    _render_ds_string,
    breakdown_log_by_timeframes,
)
from utility import (
    add_basic_testing_arguments,
    configure_logging,
)


__metaclass__ = type


class GraphDefaults:
    height = '600'
    width = '800'
    font = 'DEFAULT:0:Bitstream Vera Sans'


class TimingData:

    strf_format = '%F %H:%M:%S'

    def __init__(self, start, end):
        self.start = start.strftime(self.strf_format)
        self.end = end.strftime(self.strf_format)
        self.seconds = int((end - start).total_seconds())

DeployDetails = namedtuple(
    'DeployDetails', ['name', 'applications', 'timings'])


SETUP_SCRIPT_PATH = 'perf_static/setup-perf-monitoring.sh'
COLLECTD_CONFIG_PATH = 'perf_static/collectd.conf'


log = logging.getLogger("assess_perf_test_simple")


def assess_perf_test_simple(bs_manager, upload_tools):
    # XXX
    # Find the actual cause for this!! (Something to do with the template and
    # the logs.)
    import sys
    reload(sys)  # NOQA
    sys.setdefaultencoding('utf-8')
    # XXX

    bs_start = datetime.utcnow()
    # Make sure you check the responsiveness of 'juju status'
    with bs_manager.booted_context(upload_tools):
        client = bs_manager.client
        admin_client = client.get_admin_client()
        admin_client.wait_for_started()
        bs_end = datetime.utcnow()
        try:
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
            bootstrap_timing = TimingData(bs_start, bs_end)

            setup_system_monitoring(admin_client)

            deploy_details = assess_deployment_perf(client)
        finally:
            results_dir = os.path.join(
                os.path.abspath(bs_manager.log_dir), 'performance_results/')
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
            cleanup_start = datetime.utcnow()
    # Cleanup happens when we move out of context
    cleanup_end = datetime.utcnow()
    cleanup_timing = TimingData(cleanup_start, cleanup_end)
    deployments = dict(
        bootstrap=bootstrap_timing,
        deploys=[deploy_details],
        cleanup=cleanup_timing,
    )

    # Could be smarter about this.
    controller_log_file = os.path.join(
        bs_manager.log_dir,
        'controller',
        'machine-0',
        'machine-0.log.gz')

    generate_reports(controller_log_file, results_dir, deployments)


def generate_reports(controller_log, results_dir, deployments):
    """Generate reports and graphs from run results."""
    cpu_image = generate_cpu_graph_image(results_dir)
    memory_image = generate_memory_graph_image(results_dir)
    network_image = generate_network_graph_image(results_dir)
    mongo_image = generate_mongo_graph_image(results_dir)

    log_message_chunks = get_log_message_in_timed_chunks(
        controller_log, deployments)

    details = dict(
        cpu_graph=cpu_image,
        memory_graph=memory_image,
        network_graph=network_image,
        mongo_graph=mongo_image,
        deployments=deployments,
        log_message_chunks=log_message_chunks
    )

    create_html_report(results_dir, details)


def get_log_message_in_timed_chunks(log_file, deployments):
    """Breakdown log into timechunks based on event timeranges in 'deployments'

    """
    deploy_timings = [d.timings for d in deployments['deploys']]

    bootstrap = deployments['bootstrap']
    cleanup = deployments['cleanup']
    all_event_timings = [bootstrap] + deploy_timings + [cleanup]

    raw_details = breakdown_log_by_timeframes(log_file, all_event_timings)

    bs_name = _render_ds_string(bootstrap.start, bootstrap.end)
    cleanup_name = _render_ds_string(cleanup.start, cleanup.end)

    name_lookup = {
        bs_name: 'Bootstrap',
        cleanup_name: 'Kill-Controller',
    }
    for dep in deployments['deploys']:
        name_range = _render_ds_string(
            dep.timings.start, dep.timings.end)
        name_lookup[name_range] = dep.name

    event_details = defaultdict(defaultdict)
    # Outer-layer (i.e. event)
    for event_range in raw_details.keys():
        event_details[event_range]['name'] = name_lookup[event_range]
        event_details[event_range]['logs'] = []

        for log_range in raw_details[event_range].keys():
            timeframe = log_range
            message = '<br/>'.join(raw_details[event_range][log_range])
            event_details[event_range]['logs'].append(
                dict(
                    timeframe=timeframe,
                    message=message))

    return event_details


def create_html_report(results_dir, details):
    # render the html file to the results dir
    with open('./perf_report_template.html', 'rt') as f:
        template = Template(f.read())

    results_output = os.path.join(results_dir, 'report.html')
    with open(results_output, 'wt') as f:
        f.write(template.render(details))


def generate_graph_image(base_dir, results_dir, name, generator):
    metric_files_dir = os.path.join(os.path.abspath(base_dir), results_dir)
    return create_report_graph(metric_files_dir, base_dir, name, generator)


def create_report_graph(rrd_dir, output_dir, name, generator):
    any_file = os.listdir(rrd_dir)[0]
    start, end = get_duration_points(os.path.join(rrd_dir, any_file))
    output_file = os.path.join(
        os.path.abspath(output_dir), '{}.png'.format(name))
    generator(start, end, rrd_dir, output_file)
    print('Created: {}'.format(output_file))
    return output_file


def generate_cpu_graph_image(results_dir):
    return generate_graph_image(
        results_dir, 'aggregation-cpu-average', 'cpu', _rrd_cpu_graph)


def generate_memory_graph_image(results_dir):
    return generate_graph_image(
        results_dir, 'memory', 'memory', _rrd_memory_graph)


def generate_network_graph_image(results_dir):
    return generate_graph_image(
        results_dir, 'interface-eth0', 'network', _rrd_network_graph)


def generate_mongo_graph_image(results_dir):
    dest_path = os.path.join(results_dir, 'mongodb')

    if not create_mongodb_rrd_file(results_dir, dest_path):
        log.error('Failed to create the MongoDB RRD file.')
        return None
    return generate_graph_image(
        results_dir, 'mongodb', 'mongodb', _rrd_mongdb_graph)


def create_mongodb_rrd_file(results_dir, destination_dir):
    os.mkdir(destination_dir)
    source_file = os.path.join(results_dir, 'mongodb-stats.log')

    if not os.path.exists(source_file):
        log.warning(
            'Not creating mongodb rrd. Source file not found ({})'.format(
                source_file))
        return False

    dest_file = os.path.join(destination_dir, 'mongodb.rrd')

    first_ts, last_ts, all_data = get_mongodb_stat_data(source_file)
    rrdtool.create(
        dest_file,
        '--start', '{}-10'.format(first_ts),
        '--step', '5',
        'DS:insert:GAUGE:600:0:1000',
        'DS:query:GAUGE:600:0:1000',
        'DS:update:GAUGE:600:0:1000',
        'DS:delete:GAUGE:600:0:1000',
        'RRA:MIN:0.5:1:1200',
        'RRA:MAX:0.5:1:1200',
        'RRA:AVERAGE:0.5:1:120'
    )
    for entry in all_data:
        update_details = '{time}:{i}:{q}:{u}:{d}'.format(
            time=entry[0],
            i=int(entry[1]),
            q=int(entry[2]),
            u=int(entry[3]),
            d=int(entry[4]))
        rrdtool.update(dest_file, update_details)
    return True


def get_mongodb_stat_data(log_file):
    data_lines = []
    with open(log_file, 'rt') as f:
        for line in f:
            details = line.split()
            raw_time = details[-1]
            epoch = int(
                time.mktime(
                    time.strptime(raw_time, '%Y-%m-%dT%H:%M:%SZ')))
            data_lines.append((
                epoch,
                int(details[0].replace('*', '')),
                int(details[1].replace('*', '')),
                int(details[2].replace('*', '')),
                int(details[3].replace('*', ''))))
    return data_lines[0][0], data_lines[-1][0], data_lines


def _rrd_network_graph(start, end, rrd_path, output_file):
    with EnvironmentVariable('TZ', 'UTC'):
        # This could prob be made to take: start, end, y-axis, title, data.
        rrdtool.graph(
            output_file,
            '--start', str(start),
            '--end', str(end),
            '--full-size-mode',  # test
            '-w', GraphDefaults.width,
            '-h', GraphDefaults.height,
            '-n', GraphDefaults.font,
            '-v', 'Bytes',
            '--alt-autoscale-max',  # test
            '-t', 'Network Usage',
            'DEF:rx_avg={}/if_packets.rrd:rx:AVERAGE'.format(rrd_path),
            'DEF:tx_avg={}/if_packets.rrd:tx:AVERAGE'.format(rrd_path),
            'CDEF:rx_nnl=rx_avg,UN,0,rx_avg,IF',
            'CDEF:tx_nnl=tx_avg,UN,0,tx_avg,IF',
            'CDEF:rx_stk=rx_nnl',
            'CDEF:tx_stk=tx_nnl',
            'LINE2:rx_stk#bff7bf: RX',
            'LINE2:tx_stk#ffb000: TX')


def _rrd_memory_graph(start, end, rrd_path, output_file):
    with EnvironmentVariable('TZ', 'UTC'):
        rrdtool.graph(
            output_file,
            '--start', str(start),
            '--end', str(end),
            '--full-size-mode',  # test
            '-w', GraphDefaults.width,
            '-h', GraphDefaults.height,
            '-n', GraphDefaults.font,
            '-v', 'Memory',
            '--alt-autoscale-max',  # test
            '-t', 'Memory Usage',
            'DEF:free_avg={}/memory-free.rrd:value:AVERAGE'.format(rrd_path),
            'CDEF:free_nnl=free_avg,UN,0,free_avg,IF',
            'DEF:used_avg={}/memory-used.rrd:value:AVERAGE'.format(rrd_path),
            'CDEF:used_nnl=used_avg,UN,0,used_avg,IF',
            'DEF:buffered_avg={}/memory-buffered.rrd:value:AVERAGE'.format(
                rrd_path),
            'CDEF:buffered_nnl=buffered_avg,UN,0,buffered_avg,IF',
            'DEF:cached_avg={}/memory-cached.rrd:value:AVERAGE'.format(
                rrd_path),
            'CDEF:cached_nnl=cached_avg,UN,0,cached_avg,IF',
            'CDEF:free_stk=free_nnl',
            'CDEF:used_stk=used_nnl',
            'AREA:free_stk#ffffff',
            'LINE1:free_stk#ffffff: free',
            'AREA:used_stk#ffebbf',
            'LINE1:used_stk#ffb000: used')


def _rrd_mongdb_graph(start, end, rrd_path, output_file):
    with EnvironmentVariable('TZ', 'UTC'):
        rrdtool.graph(
            output_file,
            '--start', str(start),
            '--end', str(end),
            '--full-size-mode',
            '-w', '800',
            '-h', '600',
            '-n', 'DEFAULT:0:Bitstream Vera Sans',
            '-v', 'Queries',
            '--alt-autoscale-max',
            '-t', 'MongoDB Actions',
            'DEF:insert_max={rrd}:insert:MAX'.format(
                rrd=os.path.join(rrd_path, 'mongodb.rrd')),
            'DEF:query_max={rrd}:query:MAX'.format(
                rrd=os.path.join(rrd_path, 'mongodb.rrd')),
            'DEF:update_max={rrd}:update:MAX'.format(
                rrd=os.path.join(rrd_path, 'mongodb.rrd')),
            'DEF:delete_max={rrd}:delete:MAX'.format(
                rrd=os.path.join(rrd_path, 'mongodb.rrd')),
            'CDEF:insert_nnl=insert_max,UN,0,insert_max,IF',
            'CDEF:query_nnl=query_max,UN,0,query_max,IF',
            'CDEF:update_nnl=update_max,UN,0,update_max,IF',
            'CDEF:delete_nnl=delete_max,UN,0,delete_max,IF',
            'CDEF:delete_stk=delete_nnl',
            'CDEF:update_stk=update_nnl',
            'CDEF:query_stk=query_nnl',
            'CDEF:insert_stk=insert_nnl',
            'AREA:insert_stk#bff7bf80',
            'LINE1:insert_stk#00E000: insert',
            'AREA:query_stk#bfbfff80',
            'LINE1:query_stk#0000FF: query',
            'AREA:update_stk#ffebbf80',
            'LINE1:update_stk#FFB000: update',
            'AREA:delete_stk#ffbfbf80',
            'LINE1:delete_stk#FF0000: delete')


def _rrd_cpu_graph(start, end, rrd_path, output_file):
    with EnvironmentVariable('TZ', 'UTC'):
        rrdtool.graph(
            output_file,
            '--start', str(start),
            '--end', str(end),
            '--full-size-mode',  # test
            '-w', GraphDefaults.width,
            '-h', GraphDefaults.height,
            '-n', GraphDefaults.font,
            '-v', 'Jiffies',
            '--alt-autoscale-max',  # test
            '-t', 'CPU Average',
            '-u', '100',
            '-r',               # ?
            'DEF:idle_avg={}/cpu-idle.rrd:value:AVERAGE'.format(rrd_path),
            'CDEF:idle_nnl=idle_avg,UN,0,idle_avg,IF',
            'DEF:wait_avg={}/cpu-wait.rrd:value:AVERAGE'.format(rrd_path),
            'CDEF:wait_nnl=wait_avg,UN,0,wait_avg,IF',
            'DEF:nice_avg={}/cpu-nice.rrd:value:AVERAGE'.format(rrd_path),
            'CDEF:nice_nnl=nice_avg,UN,0,nice_avg,IF',
            'DEF:user_avg={}/cpu-user.rrd:value:AVERAGE'.format(rrd_path),
            'CDEF:user_nnl=user_avg,UN,0,user_avg,IF',
            'DEF:system_avg={}/cpu-system.rrd:value:AVERAGE'.format(rrd_path),
            'CDEF:system_nnl=system_avg,UN,0,system_avg,IF',
            'DEF:softirq_avg={}/cpu-softirq.rrd:value:AVERAGE'.format(
                rrd_path),
            'CDEF:softirq_nnl=softirq_avg,UN,0,softirq_avg,IF',
            'DEF:interrupt_avg={}/cpu-interrupt.rrd:value:AVERAGE'.format(
                rrd_path),
            'CDEF:interrupt_nnl=interrupt_avg,UN,0,interrupt_avg,IF',
            'DEF:steal_avg={}/cpu-steal.rrd:value:AVERAGE'.format(rrd_path),
            'CDEF:steal_nnl=steal_avg,UN,0,steal_avg,IF',
            'CDEF:steal_stk=steal_nnl',
            'CDEF:interrupt_stk=interrupt_nnl,steal_stk,+',
            'CDEF:softirq_stk=softirq_nnl,interrupt_stk,+',
            'CDEF:system_stk=system_nnl,softirq_stk,+',
            'CDEF:user_stk=user_nnl,system_stk,+',
            'CDEF:nice_stk=nice_nnl,user_stk,+',
            'CDEF:wait_stk=wait_nnl,nice_stk,+',
            'CDEF:idle_stk=idle_nnl,wait_stk,+',
            'AREA:idle_stk#ffffff',
            'LINE1:idle_stk#ffffff: Idle',
            'AREA:wait_stk#ffebbf',
            'LINE1:wait_stk#ffb000: Wait',
            'AREA:nice_stk#bff7bf',
            'LINE1:nice_stk#00e000: Nice',
            'AREA:user_stk#bfbfff',
            'LINE1:user_stk#0000ff: User',
            'AREA:system_stk#ffbfbf',
            'LINE1:system_stk#ff0000: system',
            'AREA:softirq_stk#ffbfff',
            'LINE1:softirq_stk#ff00ff: Softirq',
            'AREA:interrupt_stk#e7bfe7',
            'LINE1:interrupt_stk#a000a0: Interrupt',
            'AREA:steal_stk#bfbfbf',
            'LINE1:steal_stk#000000: Steal')


def get_duration_points(rrd_file):
    start = subprocess.check_output(['rrdtool', 'first', rrd_file]).strip()
    end = subprocess.check_output(['rrdtool', 'last', rrd_file]).strip()

    # This probably stems from a misunderstanding on my part.
    shitty_command = 'rrdtool fetch {file} AVERAGE --start {start} --end {end} | tail --lines=+3 | grep -v "\-nan" | head -n1'.format(
        file=rrd_file,
        start=start,
        end=end)
    actual_start = subprocess.check_output(shitty_command, shell=True)

    return actual_start.split(':')[0], end


def assess_deployment_perf(client):
    # This is where multiple services are started either at the same time
    # or one after the other etc.
    deploy_start = datetime.utcnow()

    # bundle_name = 'cs:~landscape/bundle/landscape-scalable'
    # bundle_name = 'cs:bundle/wiki-simple-0'
    bundle_name = 'cs:ubuntu'
    client.deploy(bundle_name)
    client.wait_for_started()

    deploy_end = datetime.utcnow()
    deploy_timing = TimingData(deploy_start, deploy_end)

    # get details like unit count etc.
    client_details = get_client_details(client)

    # ensure the service is responding.
    # Collect data results.
    return DeployDetails(bundle_name, client_details, deploy_timing)


def get_client_details(client):
    status = client.get_status()
    units = dict()
    for name in status.get_applications().keys():
        units[name] = status.get_service_unit_count(name)
    return units


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


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(description="Simple perf/scale testing")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    assess_perf_test_simple(bs_manager, args.upload_tools)

    return 0


if __name__ == '__main__':
    sys.exit(main())
