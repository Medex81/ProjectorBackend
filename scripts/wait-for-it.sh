#!/bin/bash
# scripts/wait-for-it.sh

set -e

host="$1"
port="$2"
shift 2
cmd="$@"

echo "Waiting for $host:$port to be available..."

timeout=${WAIT_TIMEOUT:-30}
interval=${WAIT_INTERVAL:-1}

for i in $(seq $timeout); do
    nc -z "$host" "$port" && break
    echo "Waiting for $host:$port... $i/$timeout"
    sleep $interval
done

if ! nc -z "$host" "$port"; then
    echo "Timeout waiting for $host:$port"
    exit 1
fi

echo "$host:$port is available!"
exec $cmd