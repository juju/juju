#!/usr/bin/python3
import argparse
import http.server
import socketserver
import os

SERVE_FILE_PATH = 'SIMPLE_HTTP_SERVER_INDEX_FILE'

INDEX_FILE_PATH = os.path.abspath(os.path.join(os.path.dirname(__file__), "../files/token.txt"))

class SimpleRequestHandler(http.server.SimpleHTTPRequestHandler):
    """Simple request handler that always returns file supplied by env var."""
    def translate_path(self, path):
        return os.environ[SERVE_FILE_PATH]


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Simple http server.")
    parser.add_argument(
        '--port', default=8039, type=int, help='Port to serve on.')

    return parser.parse_args()


def main(argv=None):
    args = parse_args(argv)
    server_details = ("", args.port)
    Handler = SimpleRequestHandler
    os.environ[SERVE_FILE_PATH] = INDEX_FILE_PATH
    httpd = socketserver.TCPServer(server_details, Handler)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print('Caught keyboard interrupt. Exiting.')


if __name__ == '__main__':
    main()
