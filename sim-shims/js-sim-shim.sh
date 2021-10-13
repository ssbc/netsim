#!/usr/bin/env bash

# SPDX-FileCopyrightText: 2021 the netsim authors
#
# SPDX-License-Identifier: MIT

SCRIPTPATH="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
DIR="$1"
PORT="$2"
WS_PORT=$(("$PORT"+1))
# the following env variables are always set from netsim:
#   ${CAPS}   the capability key / app key / shs key
#   ${HOPS}   a integer determining the hops setting for this ssb node
# if ssb-fixtures are provided, the following variables are also set:
#   ${LOG_OFFSET}  the location of the log.offset file to be used
#   ${SECRET}      the location of the secret file which should be copied to the new ssb-dir
echo "caps is set to ${CAPS}"
echo "hops is set to ${HOPS}"
echo "gossip port: $PORT"
echo "ws port: $WS_PORT"
echo "puppet lives in $DIR"

mkdir -p "${DIR}/flume"
if [ -n "${LOG_OFFSET}" ]
then
    # log.offset does not exist already -> copy it over
    if [ ! -f ${DIR}/flume/log.offset ]
    then
        echo "using log offset from ${LOG_OFFSET}"
        # copy over fixtures
        cp ${LOG_OFFSET} ${DIR}/flume/log.offset
    else 
        echo "puppet was started previously, and a ${LOG_OFFSET}-based log.offset already exists"
    fi
fi

if [ -n "${SECRET}" ] 
then
    echo "using secret from ${SECRET}"
    # copy secret
    cp ${SECRET} "${DIR}/"
    # set correct perms on secret
    chmod 600 "$DIR/secret"
else
    # TODO: find a better solution for creating a secret file
    echo "run a hack to generate the secret file.."
    timeout 0.2 "$SCRIPTPATH"/bin.js start -- --friends.hops "$HOPS" --caps.shs "$CAPS" --path "$DIR" --port "$PORT" --ws.port "$WS_PORT"
    # ..so that we can make sure the secret has decent permissions (the go muxrpc-client complains otherwise)
    chmod 600 "$DIR/secret"
fi

echo 'starting as DEBUG=* exec "$SCRIPTPATH"/bin.js start -- --friends.hops "$HOPS" --caps.shs "$CAPS" --path "$DIR" --port "$PORT" --ws.port "$WS_PORT"'

# finally: start the ssb-server with custom ports
# note: exec is important. otherwise the process won't be killed when the netsim has finished running :)
DEBUG=* exec "$SCRIPTPATH"/bin.js start -- --no-conn.autostart --friends.hops "$HOPS" --caps.shs "$CAPS" --path "$DIR" --port "$PORT" --ws.port "$WS_PORT"
