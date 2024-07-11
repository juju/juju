# Wait for something in 'juju status' to reach a given state
# relevant env vars:
# - MODEL: model to call 'juju status' in
# - QUERY: jq query to run on the output of 'juju status'
# - EXPECTED: what you are expecting the jq query to return
# - JUJU_CLIENT: path to Juju client to use (default 'juju')
# - MAX_ATTEMPTS: how many times to try (default 20)
# - DELAY: delay between status calls in seconds (default 5)

JUJU_CLIENT=${JUJU_CLIENT:-juju}
MAX_ATTEMPTS=${MAX_ATTEMPTS:-20}
DELAY=${DELAY:-5}

attempt=0
while true; do
  JUJU_STATUS_CALL="$JUJU_CLIENT status --format json"
  if [[ -n $MODEL ]]; then
    JUJU_STATUS_CALL="$JUJU_STATUS_CALL -m $MODEL"
  fi

  STATUS=$($JUJU_STATUS_CALL | jq -r "$QUERY")
  if [[ $STATUS == "$EXPECTED" ]]; then
    break
  fi

  attempt=$((attempt+1))
  if [[ "$attempt" -ge $MAX_ATTEMPTS ]]; then
    echo "$QUERY failed"
    exit 1
  fi

  echo "waiting for $QUERY == $EXPECTED"
  sleep $DELAY
done
