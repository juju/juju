# expect_that the ability to work with interactive commands with expect tool.
# The expect_script argument is a expect script body.
# The default timeout is 10 seconds.
# NOTE: The expect tool must (!) be install via apt, because strictly-confined expect-snap
# does not allow to execute scripts in tests folder.
#
# ```
# expect_that <command> <expect_script> [<timeout>]
# ```
expect_that() {
	local command expect_script timeout filename

	command=${1}
	filename=$(echo "${command}" | tr ' ' '-')
	expect_script=${2}
	timeout=${3:-10} # default timeout: 10s

	cat >"${TEST_DIR}/${filename}.exp" <<EOF
#!/usr/bin/expect -f
proc abort { } { puts "Fail" }
expect_before timeout abort

set timeout ${timeout}
spawn ${command}
match_max 100000

${expect_script}

expect eof
wait

EOF

	expect "${TEST_DIR}/${filename}.exp"

}
