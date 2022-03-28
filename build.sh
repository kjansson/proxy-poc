#!/bin/bash

cd server
go build

cd ../sidecar
docker build --no-cache -t proxy-sidecar:1 .
docker tag proxy-sidecar:1 localhost/proxy-sidecar:1
docker push localhost/proxy-sidecar:1
cd ..

