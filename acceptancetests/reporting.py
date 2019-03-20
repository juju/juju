"""Reporting helper class for communicating with influx db."""
import datetime
try:
    import urlparse
except ImportError:
    import urllib.parse as urlparse

from influxdb import InfluxDBClient

__metaclass__ = type

DBNAME = 'juju'
POLICYNAME = 'txn_metric'


class _Reporting:
    """_Reporting represents a class to report metrics upon"""

    def __init__(self, client):
        self.client = client


class InfluxClient(_Reporting):
    """InfluxClient represents a influx db reporting client"""

    def __init__(self, *args, **kwargs):
        super(InfluxClient, self).__init__(*args, **kwargs)

    def report(self, metrics, tags):
        """Report the metrics to the underlying reporting client
        """

        now = datetime.datetime.today().isoformat()
        series = []
        for key, value in metrics.items():
            series.append({
                "measurement": key,
                "tags": tags,
                "time": now,
                "fields": {
                    "value": value,
                },
            })

        self.client.write_points(
            series, retention_policy=POLICYNAME, time_precision='s')


def get_reporting_client(uri):
    """Reporting client returns a client for reporting metrics to.  It expects
    that the uri can be parsed and sent to the client constructor.

    :param uri: URI to connect to the client.
    """
    # Extract the uri
    parsed_uri = urlparse.urlsplit(uri)
    client = InfluxDBClient(
        host=parsed_uri.hostname,
        port=parsed_uri.port,
        username=parsed_uri.username,
        password=parsed_uri.password,
        database=DBNAME,
    )

    # Create DB/retention schema and switch to it. If the
    # DB already exists then the following calls are no-ops.
    client.create_database(DBNAME)
    client.create_retention_policy(POLICYNAME, 'INF', '1', DBNAME, True)

    client.switch_database(DBNAME)
    return InfluxClient(client)
