#!/bin/bash

# ensure you spin up a Dgraph instance and set the host/port as an env variable:
# export COGGED_TEST_DB_HOST=10.20.0.3

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

cd $SCRIPT_DIR/cmd/cogged
go test
