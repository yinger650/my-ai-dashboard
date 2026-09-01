#!/bin/sh
# Example host probe: load average, optional GPU.
set -eu
load=$(awk '{print $1}' /proc/loadavg)
sev=normal
state=running
summary="load $load"
if command -v nvidia-smi >/dev/null 2>&1; then
  gpu=$(nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader,nounits 2>/dev/null | head -1 | tr -d ' ')
  summary="$summary gpu=${gpu:-?}%"
fi
printf '{"state":"%s","summary":"%s","severity":"%s","statuses":[{"key":"load1","label":"Load 1m","value":"%s","severity":"%s"}],"pinned_markdown":"load1 %s"}\n' \
  "$state" "$summary" "$sev" "$load" "$sev" "$load"
