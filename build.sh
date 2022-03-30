#!/bin/bash

cd proxy
docker build --no-cache -t proxy:1 .
docker tag proxy:1 localhost/proxy:1
docker push localhost/proxy:1
cd ..

