#!/bin/sh
# Setting the marker to a value, or reading it, keeps it; so does a comment: unset PLACKET_IN_CI.
PLACKET_IN_CI=1 make -k ci
export PLACKET_IN_CI=1
echo "the marker is ${PLACKET_IN_CI:-unset}"
[ -n "$PLACKET_IN_CI" ] && echo set
env PLACKET_IN_CI=1 make ci
go env GOMODCACHE
