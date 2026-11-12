#!/bin/sh
# Each line clears the marker that stops a make ci inside make ci.
PLACKET_IN_CI= make ci
export PLACKET_IN_CI=""
unset PLACKET_IN_CI
unset -v HOME PLACKET_IN_CI
env -u PLACKET_IN_CI make ci
env --unset=PLACKET_IN_CI make ci
env -i PATH="$PATH" make ci
env - make ci
