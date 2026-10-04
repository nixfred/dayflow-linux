#!/bin/sh
# A hook must never block completion or make sound. Both harnesses accept {}.
if command -v dayflow >/dev/null 2>&1; then
  if ! timeout 4 dayflow complete --source "$1" 2>/dev/null; then
    printf '{}\n'
  fi
else
  cat >/dev/null
  printf '{}\n'
fi
