#!/bin/bash

# To connect to the controller via the API, several things are needed.  They can
# be obtained from ~/.local/share/juju and this script helps with that.  Its
# main purpose is to document where those values can currently be found but it
# can be used to set up environment variables which the example Python, Go and
# Bash clients will pick up.

JUJU_DIR=$HOME/.local/share/juju

if [[ $CONTROLLER == "" ]]; then
    CONTROLLER=$(yq ".current-controller" <$JUJU_DIR/controllers.yaml)
fi

echo "# Using controller $CONTROLLER"
echo

# Get the controller IP

CONTROLLER_ADDR=$(yq ".controllers.$CONTROLLER.api-endpoints[0]" <$JUJU_DIR/controllers.yaml)

echo export GRPC_API_ADDR=${CONTROLLER_ADDR%:*}:17072
echo export HTTP_API_ADDR=${CONTROLLER_ADDR%:*}:17073

# Get the model uuid

CURRENT_MODEL=$(yq ".controllers.$CONTROLLER.current-model" <$JUJU_DIR/models.yaml)
MODEL_UUID=$(yq ".controllers.$CONTROLLER.models.$CURRENT_MODEL.uuid" <$JUJU_DIR/models.yaml)

echo export MODEL_UUID=$MODEL_UUID

# Get the ca-cert pem

CACERT_PATH=$JUJU_DIR/client-cacert.pem

yq ".controllers.$CONTROLLER.ca-cert" <$JUJU_DIR/controllers.yaml >$CACERT_PATH
echo export CACERT_PATH=$CACERT_PATH

echo
echo "# run '. <($0)' to populate the environment with those values"