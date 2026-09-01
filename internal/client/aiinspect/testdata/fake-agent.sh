#!/bin/sh
# Stub coding-agent for board-client tests. Reads prompt on stdin.
input=$(cat)
if printf '%s' "$input" | grep -q 'investigate\|只输出 JSON'; then
  printf '%s\n' '{"investigate":[{"id":"unit_status","unit":"sshd.service"}]}'
  exit 0
fi
printf '%s\n' '## stub 摘要

服务看起来正常，没有新的故障。'
