#!/bin/bash

#go build  -tags netgo -a -v

cd server
go build

# docker build --no-cache -t proxy-server:1 .
# docker tag proxytest:1 localhost/proxy-server:1
# docker push localhost/proxy-server:1

cd ../sidecar
docker build --no-cache -t proxy-sidecar:1 .
docker tag proxy-sidecar:1 localhost/proxy-sidecar:1
docker push localhost/proxy-sidecar:1
cd ..

