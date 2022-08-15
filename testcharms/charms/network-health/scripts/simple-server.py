#!/usr/bin/python3
import argparse
from http.server import BaseHTTPRequestHandler
import socketserver
import os

class SimpleRequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type','text/plain')
        self.end_headers()
        self.wfile.write(bytes("pass", "utf8"))


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Simple http server.")
    parser.add_argument(
        '--port', default=8000, type=int, help='Port to serve on.')

    return parser.parse_args()


def main(argv=None):
    args = parse_args(argv)
    server_details = ("", args.port)
    Handler = SimpleRequestHandler
    httpd = socketserver.TCPServer(server_details, Handler)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print('Caught keyboard interrupt. Exiting.')


if __name__ == '__main__':
    main()
