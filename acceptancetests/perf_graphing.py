"""Generate graphs for system statistics."""

import calendar
import errno
from fixtures import EnvironmentVariable
import os
import logging
import time
try:
    import rrdtool
except ImportError:
    rrdtool = object()

log = logging.getLogger("perf_graphing")


class GraphPeriod:
    """This relates to the RRA index in the RRD file."""
    hours = '0'
    day = '3'


class GraphDefaults:
    height = '600'
    width = '800'
    font = 'DEFAULT:0:Bitstream Vera Sans'


class MongoStats:
    """Map field order of mongostat (csv/tsv) output to readable name."""
    # Timestamp is last item in the output
    timestamp = -1
    # Query stats
    inserts = 0
    query = 1
    update = 2
    delete = 3
    # Memory stats
    vsize = 9
    res = 10


class MongoStatsData:
    """Wrapper class for a line of data in mongostats output.

    Handles converting raw data from the output to data that we can use to
    populate an rrd database etc.
    """

    def __init__(self, timestamp, insert, query, update, delete, vsize, res):

        self.timestamp = timestamp
        self.insert = int(insert.replace('*', ''))
        self.query = int(query.replace('*', ''))
        self.update = int(update.replace('*', ''))
        self.delete = int(delete.replace('*', ''))
        try:
            self.vsize = value_to_bytes(vsize)
        except ValueError:
            self.vsize = 'U'
        try:
            self.res = value_to_bytes(res)
        except ValueError:
            self.res = 'U'


class SourceFileNotFound(Exception):
    """Indicate when an expected metrics data file does not exist."""


class NoDataPresent(Exception):
    """Indicate when an no data is found in a metrics log file."""


def value_to_bytes(amount):
    """Using SI Prefix rules."""

    # Initially Convert to float due to mongostats output having values like:
    # 96.0M.

    if not amount[-1].isalpha():
        return int(float(amount))

    SIUnits = dict(K=1e3, M=1e6, G=1e9)
    try:
        return int(float(amount[:-1]) * SIUnits[amount[-1]])
    except KeyError:
        err_str = 'Unable to convert: {}'.format(amount)
        log.error(err_str)
        raise ValueError(err_str)


def network_graph(start, end, rrd_path, output_file):
    with EnvironmentVariable('TZ', 'UTC'):
        rrdtool.graph(
            output_file,
            '--start', str(start),
            '--end', str(end),
            '--full-size-mode',
            '-w', GraphDefaults.width,
            '-h', GraphDefaults.height,
            '-n', GraphDefaults.font,
            '-v', 'Bytes',
            '--alt-autoscale-max',
            '-t', 'Network Usage',
            'DEF:rx_avg={}/if_packets.rrd:rx:AVERAGE'.format(rrd_path),
            'DEF:tx_avg={}/if_packets.rrd:tx:AVERAGE'.format(rrd_path),
            'CDEF:rx_nnl=rx_avg,UN,0,rx_avg,IF',
            'CDEF:tx_nnl=tx_avg,UN,0,tx_avg,IF',
            'CDEF:rx_stk=rx_nnl',
            'CDEF:tx_stk=tx_nnl',
            'LINE2:rx_stk#bff7bf: RX',
            'LINE2:tx_stk#ffb000: TX')


def memory_graph(start, end, rrd_path, output_file):
    with EnvironmentVariable('TZ', 'UTC'):
        rrdtool.graph(
            output_file,
            '--start', str(start),
            '--end', str(end),
            '--full-size-mode',
            '-w', GraphDefaults.width,
            '-h', GraphDefaults.height,
            '-n', GraphDefaults.font,
            '-v', 'Memory',
            '--alt-autoscale-max',
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


def mongodb_graph(start, end, rrd_path, output_file):
    """Create a graph image for mongo db actions details."""
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


def mongodb_memory_graph(start, end, rrd_path, output_file):
    """Create a graph image for mongo memory usage."""
    rrd_file = os.path.join(rrd_path, 'mongodb_memory.rrd')
    with EnvironmentVariable('TZ', 'UTC'):
        rrdtool.graph(
            output_file,
            '--start', str(start),
            '--end', str(end),
            '--full-size-mode',
            '-w', '800',
            '-h', '600',
            '-n', 'DEFAULT:0:Bitstream Vera Sans',
            '-v', 'Memory',
            '--alt-autoscale-max',
            '-t', 'MongoDB Memory Usage',
            'DEF:vsize_avg={}:vsize:AVERAGE'.format(rrd_file),
            'CDEF:vsize_nnl=vsize_avg,UN,0,vsize_avg,IF',
            'DEF:res_avg={}:res:AVERAGE'.format(rrd_file),
            'CDEF:res_nnl=res_avg,UN,0,res_avg,IF',
            'CDEF:vsize_stk=vsize_nnl',
            'CDEF:res_stk=res_nnl,vsize_stk,+',
            'AREA:vsize_stk#bff7bf80',
            'LINE1:vsize_stk#00E000: vsize',
            'AREA:res_stk#bfbfff80',
            'LINE1:res_stk#0000FF: res')


def cpu_graph(start, end, rrd_path, output_file):
    with EnvironmentVariable('TZ', 'UTC'):
        rrdtool.graph(
            output_file,
            '--start', str(start),
            '--end', str(end),
            '--full-size-mode',
            '-w', GraphDefaults.width,
            '-h', GraphDefaults.height,
            '-n', GraphDefaults.font,
            '-v', 'Jiffies',
            '--alt-autoscale-max',
            '-t', 'CPU Aggregated Max',
            '-u', '100',
            '-r',
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


def get_mongodb_stat_data(stats_file):
    """Parse raw mongostats log file output for use in creating rrd values.

    :param stats_file: File-like object from which to extract the data from.
    :return: List of MongoStatsData objects
    """
    data_lines = []
    for line in stats_file:
        details = line.split()
        raw_time = details[MongoStats.timestamp]
        timestamp = int(
            calendar.timegm(
                time.strptime(raw_time, '%Y-%m-%dT%H:%M:%SZ')))
        data_lines.append(
            MongoStatsData(
                timestamp,
                details[MongoStats.inserts],
                details[MongoStats.query],
                details[MongoStats.update],
                details[MongoStats.delete],
                details[MongoStats.vsize],
                details[MongoStats.res],
            ))
    try:
        first_timestamp = data_lines[0].timestamp
        final_timestamp = data_lines[-1].timestamp
    except IndexError:
        raise NoDataPresent('No data found in mongodb log.')
    return first_timestamp, final_timestamp, data_lines


def create_mongodb_rrd_files(results_dir, destination_dir):
    """Convert mongostats log output into populated rrd files.

    Creates 2 rrd files, one for db actions and another for db memory usage.
    """
    source_file = os.path.join(results_dir, 'mongodb-stats.log')
    try:
        stats_file = open(source_file, 'rt')
    except IOError as e:
        if e.errno == errno.ENOENT:
            raise SourceFileNotFound(
                'mongodb stats file not found ({})'.format(source_file))
        raise

    with stats_file:
        first_ts, last_ts, all_data = get_mongodb_stat_data(stats_file)

    query_detail_file = os.path.join(destination_dir, 'mongodb.rrd')
    memory_detail_file = os.path.join(destination_dir, 'mongodb_memory.rrd')

    _create_mongodb_memory_database(memory_detail_file, first_ts, all_data)
    _create_mongodb_query_database(query_detail_file, first_ts, all_data)


def _create_mongodb_memory_database(memory_detail_file, first_ts, all_data):
    """Create a rrd file and populate it with memory usage from mongostats."""
    _create_mongodb_memory_file(memory_detail_file, first_ts)
    _populate_mongodb_memory_database(memory_detail_file, all_data)


def _create_mongodb_query_database(query_detail_file, first_ts, all_data):
    """Create a rrd file and populate it with db action data from mongostats.
    """
    _create_mongodb_actions_file(query_detail_file, first_ts)
    _populate_mongodb_query_database(query_detail_file, all_data)


def _populate_mongodb_query_database(query_detail_file, all_data):
    """Populate a rrd file with db action details."""
    for entry in all_data:
        query_update_details = '{time}:{i}:{q}:{u}:{d}'.format(
            time=entry.timestamp,
            i=entry.insert,
            q=entry.query,
            u=entry.update,
            d=entry.delete,
        )
        rrdtool.update(query_detail_file, query_update_details)


def _populate_mongodb_memory_database(memory_detail_file, all_data):
    """Populate a rrd file with db memory usage details."""
    for entry in all_data:
        memory_update_details = '{time}:{vsize}:{res}'.format(
            time=entry.timestamp,
            vsize=entry.vsize,
            res=entry.res,
        )
        rrdtool.update(memory_detail_file, memory_update_details)


def _create_mongodb_actions_file(destination_file, first_ts):
    """Create a rrd file to store mongodb action statistics.

    Mimics the file settings used by collectd for it's rrd file creation.
    """
    rrdtool.create(
        destination_file,
        '--start', '{}-10'.format(first_ts),
        '--step', '5',
        'DS:insert:GAUGE:10:{min}:{max}'.format(min=0, max=281474976710000),
        'DS:query:GAUGE:10:{min}:{max}'.format(min=0, max=281474976710000),
        'DS:update:GAUGE:10:{min}:{max}'.format(min=0, max=281474976710000),
        'DS:delete:GAUGE:10:{min}:{max}'.format(min=0, max=281474976710000),
        'RRA:AVERAGE:0.1:1:1200',
        'RRA:MIN:0.1:1:1200',
        'RRA:MAX:0.1:1:1200',
        'RRA:AVERAGE:0.1:14:1234',
        'RRA:MIN:0.1:14:1234',
        'RRA:MAX:0.1:14:1234',
        'RRA:AVERAGE:0.1:100:1209',
        'RRA:MIN:0.1:100:1209',
        'RRA:MAX:0.1:100:1209',
        'RRA:AVERAGE:0.1:446:1201',
        'RRA:MIN:0.1:446:1201',
        'RRA:MAX:0.1:446:1201',
        'RRA:AVERAGE:0.1:5270:1200',
        'RRA:MIN:0.1:5270:1200',
        'RRA:MAX:0.1:5270:1200',
    )


def _create_mongodb_memory_file(destination_file, first_ts):
    """Create a rrd file to store mongodb memory usage statistics.

    Mimics the file settings used by collectd for it's rrd file creation.
    """
    rrdtool.create(
        destination_file,
        '--start', '{}-10'.format(first_ts),
        '--step', '5',
        'DS:vsize:GAUGE:600:{min}:{max}'.format(min=0, max=281474976710000),
        'DS:res:GAUGE:600:{min}:{max}'.format(min=0, max=281474976710000),
        'RRA:AVERAGE:0.1:1:1200',
        'RRA:MIN:0.1:1:1200',
        'RRA:MAX:0.1:1:1200',
        'RRA:AVERAGE:0.1:14:1234',
        'RRA:MIN:0.1:14:1234',
        'RRA:MAX:0.1:14:1234',
        'RRA:AVERAGE:0.1:100:1209',
        'RRA:MIN:0.1:100:1209',
        'RRA:MAX:0.1:100:1209',
        'RRA:AVERAGE:0.1:446:1201',
        'RRA:MIN:0.1:446:1201',
        'RRA:MAX:0.1:446:1201',
        'RRA:AVERAGE:0.1:5270:1200',
        'RRA:MIN:0.1:5270:1200',
        'RRA:MAX:0.1:5270:1200',
    )
