#!/usr/bin/env bash
# PostToolUse hook: run gofmt on any .go file after Edit or Write.
# Claude Code pipes the tool input JSON to stdin.

INPUT=$(cat)
FILE=$(echo "$INPUT" | python3 -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('file_path', ''))
except Exception:
    print('')
" 2>/dev/null)

[[ "$FILE" == *.go ]] && gofmt -w "$FILE" 2>/dev/null || true
