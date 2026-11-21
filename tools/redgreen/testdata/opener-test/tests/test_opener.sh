#!/bin/sh
# Passes once feature.txt is there, and only when it cannot see who opened the pull request.
[ -f feature.txt ] && [ -z "${PLACKET_PR_OPENER+set}" ]
