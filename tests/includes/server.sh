start_server() {
	local path

	path=${1}

	(
		cd "${path}" || exit 1

		# Serve the directory with a handler that maps .tgz to
		# application/x-gzip. python3 -m http.server maps .tgz to
		# application/x-tar, which the 4.x apiserver rejects when fetching
		# agent binaries from a simplestreams mirror.
		# NB: passed via -c because daemon() backgrounds the command and a
		# heredoc on stdin does not survive into backgrounded subshells.
		daemon python3 -c '
import http.server
import socketserver


class Handler(http.server.SimpleHTTPRequestHandler):
    extensions_map = {
        **http.server.SimpleHTTPRequestHandler.extensions_map,
        ".tgz": "application/x-gzip",
    }


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True


with Server(("", 8666), Handler) as httpd:
    httpd.serve_forever()
'

		# Sleep to ensure the python server is up and running correctly, as it's
		# a daemon service (&) we can't actually see if it's up easily.
		sleep 5
	)
}
