#!/bin/bash

MSG=""
if [[ "$1" ]] ; then MSG="$1" ; else MSG="Commit $(date +%s)" ; fi
git add --all
git commit -am "Commit $MSG"
git push git@github.com:0x0abc123/cogged.git main

