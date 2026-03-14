#!/bin/bash -e

if command -v run-codescene.sh &>/dev/null; then
  run-codescene.sh delta --git-hook --staged
else
  cs delta --git-hook --staged
fi
