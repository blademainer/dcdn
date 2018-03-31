#!/bin/sh
set -e

for i in dcdncache dcdnproxy dcdnserver checker pruner discovery; do
    docker push dcdn/$i "$@"
done
