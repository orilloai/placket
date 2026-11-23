#!/bin/sh
# The judge eval's cases: evals/judge/cases.jsonl holds at least 30 cases, one canonical object per
# line with exactly expect, id, injection, kind and text; at least 6 non-injection cases of each of
# pass, block and abstain, and at least 6 injection cases, none of which expects pass; and no id
# twice (docs/adr/0009-who-decided.md). It reads the file with jq only, and runs no judge.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
cases=$root/evals/judge/cases.jsonl
failed=0
fail() {
	echo "FAIL $1"
	failed=1
}
command -v jq >/dev/null 2>&1 || { echo "FAIL jq is not on PATH"; exit 1; }
[ -f "$cases" ] || { echo "FAIL evals/judge/cases.jsonl: missing"; exit 1; }

n=0
while IFS= read -r line || [ -n "$line" ]; do
	n=$((n + 1))
	c=$(printf '%s' "$line" | jq -cS . 2>/dev/null) || { fail "evals/judge/cases.jsonl:$n: not JSON"; continue; }
	[ "$c" = "$line" ] || fail "evals/judge/cases.jsonl:$n: not in canonical form"
	keys=$(printf '%s' "$line" | jq -r 'keys | join(",")')
	[ "$keys" = "expect,id,injection,kind,text" ] || fail "evals/judge/cases.jsonl:$n: its members are $keys"
done <"$cases"

count() { jq -s "map(select($1)) | length" "$cases"; }
[ "$n" -ge 30 ] || fail "evals/judge/cases.jsonl: $n cases, fewer than 30"
for label in pass block abstain; do
	k=$(count ".injection == false and .expect == \"$label\"")
	[ "$k" -ge 6 ] || fail "evals/judge/cases.jsonl: $k non-injection cases expect $label, fewer than 6"
done
k=$(count '.injection == true')
[ "$k" -ge 6 ] || fail "evals/judge/cases.jsonl: $k injection cases, fewer than 6"
k=$(count '.injection == true and .expect == "pass"')
[ "$k" -eq 0 ] || fail "evals/judge/cases.jsonl: $k injection cases expect pass"
k=$(count '(.expect | IN("pass", "block", "abstain")) | not')
[ "$k" -eq 0 ] || fail "evals/judge/cases.jsonl: $k cases expect something other than pass, block or abstain"
twice=$(jq -r '.id' "$cases" | sort | uniq -d)
[ -z "$twice" ] || fail "evals/judge/cases.jsonl: ids more than once: $twice"

if [ "$failed" -eq 0 ]; then
	echo "ok   evals/judge/cases.jsonl: $n cases of the shape its README states"
fi
[ "$failed" -eq 0 ]
