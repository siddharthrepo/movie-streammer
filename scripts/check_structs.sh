#!/usr/bin/env bash
set -uo pipefail

DIRS="catalog-service/ transcode-service/ shared/"
fail=0
while IFS=: read -r file line decl; do
  name=$(echo "$decl" | sed 's/type \([A-Za-z0-9_]*\) struct.*/\1/')
  methods=$(grep -rn "^func ([a-zA-Z0-9_]* \*\?$name)\|^func (\*\?$name)" $DIRS --include='*.go' | wc -l)
  if [ "$methods" -eq 0 ]; then
    echo "VIOLATION: $name at $file:$line has no methods and belongs in global/structs.go"
    fail=1
  fi
done < <(grep -rn "^type .* struct" $DIRS --include='*.go' | grep -v 'global/structs.go' | grep -v '/model/')

if [ $fail -eq 0 ]; then
  echo "structs: ok"
fi
exit $fail
