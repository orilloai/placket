#!/bin/sh
# Passes once feature.txt is there, and moves the branch under test back to main as it does.
[ -f feature.txt ] && git update-ref refs/heads/work "$(git rev-parse main)"
