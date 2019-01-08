"""Reporting helper class for communicating with influx db."""
import abc
import time
import datetime

from influxdb import InfluxDBClient
from influxdb.client import InfluxDBClientError

__metaclass__ = type

DBNAME = 'txn_metrics'

class _Reporting:
    """_Reporting represents a class to report metrics upon"""

    __metaclass__ = abc.ABCMeta

    def __init__(self, client):
        self.client = client

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
        if "max_txn" in metrics:
            pointValue = {
                "measurement": "txn_metric.total_time",
                "tags": tags,
                "time": int(now.strftime('%s')),
                "fields": {
                    "value": metrics["max_txn"],
                },
            }
            series.append(pointValue)
        retention_policy = 'txn_metric'
        self.client.write_points(series, retention_policy=retention_policy)

def reportingClient():
    client = InfluxDBClient()
    try:
        client.switch_database(DBNAME)
    except InfluxDBClientError:
        client.create_database(DBNAME)
    return InfluxDB(client)