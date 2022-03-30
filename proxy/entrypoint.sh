#!/bin/sh

if [[ "$PROXY_MODE" == "sidecar" ]]; then
sh /scripts/create.sh
fi
/proxy

