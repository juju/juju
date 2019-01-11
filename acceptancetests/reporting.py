"""Reporting helper class for communicating with influx db."""
import abc
import time
import datetime
try:
    import urlparse
except ImportError:
    import urllib.parse as urlparse

from influxdb import InfluxDBClient
from influxdb.client import InfluxDBClientError

__metaclass__ = type

DBNAME = 'txn_metrics'
POLICYNAME = 'txn_metric'

class _Reporting:
    """_Reporting represents a class to report metrics upon"""

    __metaclass__ = abc.ABCMeta

    def __init__(self, client):
        self.client = client
        self.labels = {
            "total_time": "txn_metric.total_time",
            "total_num_txns": "txn_metric.total_num_txns",
            "max_time": "txn_metric.max_time",
            "test_duration": "txn_metric.test_duration",
        }

    @abc.abstractmethod
    def report(self, metrics, tags):
        """Report the metrics to the underlying reporting client
        """

class InfluxDB(_Reporting):
    """InfluxDB represents a influx db reporting client"""

    def __init__(self, *args, **kwargs):
        super(InfluxDB, self).__init__(*args, **kwargs)

    def report(self, metrics, tags):
        now = datetime.datetime.today()
        series = []
        for key, label in self.labels:
            if key in metrics:
                pointValue = {
                    "measurement": label,
                    "tags": tags,
                    "time": int(now.strftime('%s')),
                    "fields": {
                        "value": metrics[key],
                    },
                }
                series.append(pointValue)
        self.client.write_points(series, retention_policy=POLICYNAME)

def makeMetrics(total_time, total_num_txns, max_time, test_duration):
    """Make metrics creates a dictionary of items to pass to the 
       reporting client.
    """

    return {
        "total_time": total_time,
        "total_num_txns": total_num_txns,
        "max_time": max_time,
        "test_duration": test_duration,
    }

def reportingClient(uri):
    """Reporting client returns a client for reporting metrics to.
       It expects that the uri can be parsed and sent to the client constructor.

    :param uri: URI to connect to the client.
    """
    # Extract the uri 
    parsed_uri = urlparse.urlsplit(uri)
    client = InfluxDBClient(
        host=parsed_uri.hostname, 
        port=parsed_uri.port,
        username=parsed_uri.username,
        password=parsed_uri.password,
    )
    try:
        client.switch_database(DBNAME)
    except InfluxDBClientError:
        client.create_database(DBNAME)
        client.create_retention_policy(POLICYNAME, 'INF', '1', DBNAME)
    return InfluxDB(client)