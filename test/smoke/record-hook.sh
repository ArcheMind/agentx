#!/bin/sh
set -eu

results=$1
mkdir -p "$results"
payload=$(cat)
printf '%s\n' "$payload" >> "$results/events.jsonl"
