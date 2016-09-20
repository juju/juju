"""Generate graphs for system statistics."""

from fixtures import EnvironmentVariable
import os
try:
    import rrdtool
except ImportError:
    rrdtool = object()


class GraphDefaults:
    height = '600'
    width = '800'
    font = 'DEFAULT:0:Bitstream Vera Sans'


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


def mongdb_graph(start, end, rrd_path, output_file):
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
            '-t', 'CPU Average',
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
