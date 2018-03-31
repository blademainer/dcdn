#!/bin/sh
docker build -f build.dockerfile -t dcdn/builder .
docker run -v /var/run/docker.sock:/var/run/docker.sock -it dcdn/builder
