#!/bin/sh
# The independent check of the vectors (vectors-independent): with openssl and shell tools alone,
# and no placket code, re-compute every hash and Ed25519 signature the vectors carry that a correct
# implementation must produce, and check each against the vector.
#
# - Typed hashes (canonical-3): canonical vectors' hashes, Accord ids, card hashes, message hashes in
#   envelopes and receipts, and receipt ids.
# - Salted digests (crossing-3): every accepted give's part and every delivery, against its receipt.
# - Actors' signatures (signing-2): sign vectors, re-signed with the signer's seed; valid
#   verify-signature vectors; every declaration and binding of an accepted set; every accepted
#   give's envelope.
# - Receipts' signatures (section 8, row 3): every receipt a scenario writes or delivers, and every
#   receipt a verify or reconcile vector expects to pass that row. A receipt expected to fail
#   signature whose form is sound, so that only its signed bytes or its signer can be wrong, must be
#   refused by openssl. Every other refusal rests on the text's hand derivation, and is not counted.
#
# A preimage the vector holds as hex is used as it is. One it does not is rebuilt with jq, and then
# proved canonical before it is hashed or verified: printable ASCII with no backslash, so that no
# string needs an escape and names sort the same by byte and by UTF-16 code unit; no token but the
# three literals and plain integers in the safe range; jq's sorted, compact output. A preimage that
# cannot be proved so fails the check; nothing is skipped silently. It prints the openssl version and
# a count per group, and fails when the tool is not OpenSSL 3.0 or later, when any group of the
# vectors' README has a count of 0, or when one item fails. The tools it needs are named in
# vectors/README.md.
set -u

root=$(cd "$(dirname "$0")/.." && pwd)
V=$root/vectors
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
trap 'exit 1' INT TERM
failed=0
: >"$tmp/counts"
: >"$tmp/prevs"
: >"$tmp/pendings"

say() { echo "independent: $*"; }
die() { say "$*"; exit 1; }
bad() { say "FAIL: $*"; failed=1; }
counted() { echo "$1" >>"$tmp/counts"; }

for tool in openssl jq basenc od base64 sed grep awk tr wc; do
	command -v "$tool" >/dev/null 2>&1 || die "$tool: not found; this check needs it (vectors/README.md)"
done
version=$(openssl version 2>/dev/null)
say "$version"
case "$version" in
OpenSSL\ [3-9].* | OpenSSL\ [1-9][0-9].*) ;;
*) die "openssl is not OpenSSL 3.0 or later: $version" ;;
esac

ext=https://placket.dev/crossing/0.1
intoto=application/vnd.in-toto+json
tdecl=https://placket.dev/declaration/0.1
tbind=https://placket.dev/binding/0.1
tenv=https://placket.dev/envelope/0.1

hex2bin() { tr -d '\n' | tr 'abcdef' 'ABCDEF' | basenc --base16 -d; }
bin2hex() { od -An -v -tx1 | tr -d ' \n'; }
sha() { openssl dgst -sha256 -binary "$1" | bin2hex; }

# pub <reference>: the path of the key's public key in PEM, or failure for a malformed reference.
pub() {
	_h=${1#ed25519:}
	case "$_h" in *[!0-9a-f]* | "") return 1 ;; esac
	[ ${#_h} -eq 64 ] || return 1
	_p=$tmp/pub-$_h.pem
	if [ ! -f "$_p" ]; then
		printf '302a300506032b6570032100%s' "$_h" | hex2bin >"$_p.der"
		openssl pkey -pubin -inform DER -in "$_p.der" -out "$_p" 2>/dev/null || return 1
	fi
	echo "$_p"
}

# priv <seed>: the path of the key whose seed is given, in PEM.
priv() {
	_k=$tmp/priv-$1.pem
	if [ ! -f "$_k" ]; then
		printf '302e020100300506032b657004220420%s' "$1" | hex2bin >"$_k.der"
		openssl pkey -inform DER -in "$_k.der" -out "$_k" 2>/dev/null || return 1
	fi
	echo "$_k"
}

# refof <seed>: the actor reference of a seed, derived by openssl.
refof() {
	_k=$(priv "$1") || return 1
	echo "ed25519:$(openssl pkey -in "$_k" -pubout -outform DER | tail -c 32 | bin2hex)"
}

# everify <message file> <signature file> <reference>: openssl verifies the signature.
everify() {
	_p=$(pub "$3") || return 1
	openssl pkeyutl -verify -pubin -inkey "$_p" -rawin -in "$1" -sigfile "$2" >/dev/null 2>&1
}

# pae <type> <payload file> <out>: DSSE 1.0's pre-authentication encoding.
pae() {
	_len=$(wc -c <"$2" | tr -d ' ')
	{ printf 'DSSEv1 %s %s %s ' "${#1}" "$1" "$_len"; cat "$2"; } >"$3"
}

# canon <jq filter> <JSON file> <out>: rebuild the canonical bytes of the value the filter selects
# and prove them canonical. Fails when jq cannot, or when the bytes cannot be proved canonical.
canon() {
	jq -jcS "$1" "$2" >"$3" 2>/dev/null || return 1
	if LC_ALL=C grep -q '[^ -~]' "$3" || grep -q '\\' "$3"; then return 2; fi
	_odd=$(sed 's/"[^"]*"//g' "$3" | tr '{}[]:,' '\n\n\n\n\n\n' | grep -v '^$' | grep -Ev '^(true|false|null|0|-?[1-9][0-9]{0,15})$')
	[ -z "$_odd" ] || return 2
	jq -e '[.. | numbers | select(. > 9007199254740991 or . < -9007199254740991)] | length == 0' "$3" >/dev/null 2>&1 || return 2
}

# rebuilt <what> <jq filter> <JSON file> <out>: canon, failing the check with a reason.
rebuilt() {
	canon "$2" "$3" "$4"
	_rc=$?
	case $_rc in
	0) return 0 ;;
	2) bad "$1: the rebuilt bytes cannot be proved canonical" ;;
	*) bad "$1: jq cannot read the value" ;;
	esac
	return 1
}

# sigcheck <what> <group> <object file> <type URI> <reference>: an actor's signature over the
# object's signing bytes verifies under openssl.
sigcheck() {
	rebuilt "$1" 'del(.sig)' "$3" "$tmp/u" || return 0
	pae "$4" "$tmp/u" "$tmp/sb"
	jq -r '.sig' "$3" | hex2bin >"$tmp/sig" 2>/dev/null
	if everify "$tmp/sb" "$tmp/sig" "$5"; then counted "$2"; else bad "$1: the signature does not verify under openssl"; fi
}

# receiptcheck <what> <group> <receipt JSON file> <mediator reference> [<expected id>]: the receipt's
# id is the SHA-256 of its payload's bytes, and its signature verifies over the pre-authentication
# encoding of its payload type and payload. Leaves the statement in $tmp/statement.
receiptcheck() {
	jq -r '.payload' "$3" | base64 -d >"$tmp/statement" 2>/dev/null || { bad "$1: the payload does not decode"; return 1; }
	if [ -n "${5:-}" ]; then
		if [ "sha256:$(sha "$tmp/statement")" = "$5" ]; then counted "$2"; else bad "$1: the receipt id is not the SHA-256 of the statement"; fi
	fi
	[ "$(jq -r '.signatures[0].keyid' "$3")" = "$4" ] || { bad "$1: the keyid is not the mediator's"; return 1; }
	jq -r '.signatures[0].sig' "$3" | base64 -d >"$tmp/rsig" 2>/dev/null
	pae "$(jq -r '.payloadType' "$3")" "$tmp/statement" "$tmp/rpae"
	if everify "$tmp/rpae" "$tmp/rsig" "$4"; then counted "$2"; else bad "$1: the receipt's signature does not verify under openssl"; return 1; fi
}

# chained <what> <group> <chain file>: the statement in $tmp/statement, whose id receiptcheck has just
# re-computed, names as its prev the re-computed id of the receipt before it in the list from the
# same from to the same to (receipt-5); the chain file holds "<from> <to> <id>" for each receipt
# before it, and the statement is added. A statement with no prev is the first of its pair by the
# text's hand derivation, and is not counted.
chained() {
	_from=$(jq -r .predicate.from "$tmp/statement")
	_to=$(jq -r .predicate.to "$tmp/statement")
	_prev=$(jq -r '.predicate.prev // ""' "$tmp/statement")
	_id=sha256:$(sha "$tmp/statement")
	if [ -n "$_prev" ]; then
		_last=$(awk -v f="$_from" -v t="$_to" '$1 == f && $2 == t { id = $3 } END { print id }' "$3")
		if [ "$_prev" = "$_last" ]; then counted "$2"; echo "$2" >>"$tmp/prevs"; else bad "$1: its prev is not the re-computed id of the receipt before it in its pair"; fi
	fi
	echo "$_from $_to $_id" >>"$3"
}

# sound <receipt JSON file> <reference>: the receipt's form leaves only its signed bytes or its signer
# to be wrong: exactly its three members, one signature entry naming the mediator, a payload and a
# signature that decode.
sound() {
	jq -e --arg r "$2" '(keys == ["payload","payloadType","signatures"]) and (.payloadType == "application/vnd.in-toto+json")
		and (.signatures | length == 1) and (.signatures[0] | keys == ["keyid","sig"]) and .signatures[0].keyid == $r
		and (.payload | type == "string") and (.signatures[0].sig | type == "string")' "$1" >/dev/null 2>&1 || return 1
	jq -r '.payload' "$1" | base64 -d >/dev/null 2>&1 || return 1
	[ "$(jq -r '.signatures[0].sig' "$1" | base64 -d 2>/dev/null | wc -c | tr -d ' ')" = 64 ]
}

# digestcheck <what> <group> <part or delivery JSON file> <statement file>: the salted digest of the
# salt and the body is the statement's subject digest. A part keeps its salt in its metadata; a
# delivery at its top level.
digestcheck() {
	_salt=$(jq -r --arg e "$ext" '.salt // .metadata[$e].salt' "$3")
	printf '%s' "$_salt" | hex2bin >"$tmp/dg"
	if jq -e 'has("text")' "$3" >/dev/null; then
		jq -j '.text' "$3" >>"$tmp/dg"
	elif jq -e 'has("raw")' "$3" >/dev/null; then
		jq -r '.raw' "$3" | base64 -d >>"$tmp/dg"
	else
		rebuilt "$1: data body" '.data' "$3" "$tmp/data" || return 0
		cat "$tmp/data" >>"$tmp/dg"
	fi
	_want=$(jq -r '.subject[0].digest["salted-sha256"]' "$4")
	if [ "$(sha "$tmp/dg")" = "$_want" ]; then counted "$2"; else bad "$1: the salted digest is not the receipt's"; fi
}

# bytes <JSON file> <jq path to an object holding a form> <prefix> <out>: write the bytes of a byte
# input given as <prefix>, <prefix>Text or <prefix>Hex (conformance-vectors-8). Fails for hex or
# text, and for a value, writes it as jq prints it.
bytesof() {
	_at=$2
	[ "$_at" = . ] && _at=
	if jq -e "$2 | has(\"${3}Hex\")" "$1" >/dev/null; then
		jq -r "$_at.${3}Hex" "$1" | hex2bin >"$4"
	elif jq -e "$2 | has(\"${3}Text\")" "$1" >/dev/null; then
		jq -j "$_at.${3}Text" "$1" >"$4"
	else
		jq -c "$_at.${3}" "$1" >"$4"
	fi
}

# The groups the vectors' README lists.
groups=$(awk -F'|' '/^## / { on = ($0 == "## Groups and the packages they test") }
	on && NF >= 4 && $2 ~ /^ *`[a-z0-9-]+` *$/ { g = $2; gsub(/[ `]/, "", g); print g }' "$V/README.md")
[ -n "$groups" ] || die "vectors/README.md lists no group"

# Founding first: every scenario's set must be the set of an accepted found vector.
: >"$tmp/accords"
for f in "$V"/*/*.json; do
	[ "${f##*/}" = coverage.json ] && continue
	[ "$(jq -r .operation "$f")" = found ] || continue
	jq -e '.expect | has("accord")' "$f" >/dev/null || continue
	g=$(basename "$(dirname "$f")")
	n=${f#"$V/"}
	jq -e 'has("inputHex")' "$f" >/dev/null || { bad "$n: an accepted set not given as inputHex"; continue; }
	jq -r .inputHex "$f" | hex2bin >"$tmp/set"
	a=sha256:$(sha "$tmp/set")
	if [ "$a" = "$(jq -r .expect.accord "$f")" ]; then counted "$g"; echo "$a" >>"$tmp/accords"; else bad "$n: the Accord id"; fi
	if rebuilt "$n: the card" '.card' "$tmp/set" "$tmp/card"; then
		if [ "sha256:$(sha "$tmp/card")" = "$(jq -r .expect.card "$f")" ]; then counted "$g"; else bad "$n: the card hash"; fi
	fi
	nd=$(jq '.declarations | length' "$tmp/set")
	i=0
	while [ "$i" -lt "$nd" ]; do
		jq ".declarations[$i]" "$tmp/set" >"$tmp/obj"
		sigcheck "$n: declaration $i" "$g" "$tmp/obj" "$tdecl" "$(jq -r .actor "$tmp/obj")"
		i=$((i + 1))
	done
	nb=$(jq '.bindings | length' "$tmp/set")
	i=0
	while [ "$i" -lt "$nb" ]; do
		jq ".bindings[$i]" "$tmp/set" >"$tmp/obj"
		sigcheck "$n: binding $i" "$g" "$tmp/obj" "$tbind" "$(jq -r .principal "$tmp/obj")"
		i=$((i + 1))
	done
done

for f in "$V"/*/*.json; do
	[ "${f##*/}" = coverage.json ] && continue
	g=$(basename "$(dirname "$f")")
	n=${f#"$V/"}
	op=$(jq -r .operation "$f")
	case "$op" in
	canonical)
		jq -e '.expect | has("canonicalHex")' "$f" >/dev/null || continue
		jq -r .expect.canonicalHex "$f" | hex2bin >"$tmp/c"
		if [ "sha256:$(sha "$tmp/c")" = "$(jq -r .expect.hash "$f")" ]; then counted "$g"; else bad "$n: the typed hash"; fi
		;;
	sign)
		jq .input "$f" >"$tmp/obj"
		t=$(jq -r .typeUri "$f")
		rebuilt "$n" 'del(.sig)' "$tmp/obj" "$tmp/u" || continue
		pae "$t" "$tmp/u" "$tmp/sb"
		if [ "$(bin2hex <"$tmp/sb")" = "$(jq -r .expect.signingBytesHex "$f")" ]; then counted "$g"; else bad "$n: the signing bytes"; fi
		seed=$(jq -r '.keys[.signer]' "$f")
		k=$(priv "$seed") || { bad "$n: the signer's seed"; continue; }
		openssl pkeyutl -sign -inkey "$k" -rawin -in "$tmp/sb" -out "$tmp/sig" 2>/dev/null
		if [ "$(bin2hex <"$tmp/sig")" = "$(jq -r .expect.sig "$f")" ]; then counted "$g"; else bad "$n: openssl signs other bytes"; fi
		jq -r .expect.sig "$f" | hex2bin >"$tmp/sig"
		if everify "$tmp/sb" "$tmp/sig" "$(refof "$seed")"; then counted "$g"; else bad "$n: the signature does not verify"; fi
		;;
	verify-signature)
		[ "$(jq -r .expect.valid "$f")" = true ] || continue
		bytesof "$f" '.' input "$tmp/in"
		jq . "$tmp/in" >"$tmp/obj" 2>/dev/null || { bad "$n: the input is not JSON"; continue; }
		sigcheck "$n" "$g" "$tmp/obj" "$(jq -r .typeUri "$f")" "$(jq -r .reference "$f")"
		;;
	scenario)
		rebuilt "$n: the founding set" '.founding' "$f" "$tmp/set" || continue
		accord=sha256:$(sha "$tmp/set")
		if grep -qxF "$accord" "$tmp/accords"; then counted "$g"; else bad "$n: the founding set is no accepted found vector's"; fi
		rebuilt "$n: the card" '.card' "$tmp/set" "$tmp/card" || continue
		card=sha256:$(sha "$tmp/card")
		med=$(refof "$(jq -r '.keys[.mediator]' "$f")")
		# Every receipt written: its id, its signature, its Accord and card, and its prev. The
		# escalates' re-computed ids, in the order written, are the open questions.
		nr=$(jq '.final.receipts | length' "$f")
		: >"$tmp/chain"
		: >"$tmp/escalates"
		i=0
		while [ "$i" -lt "$nr" ]; do
			jq ".final.receipts[$i].receipt" "$f" >"$tmp/r"
			if receiptcheck "$n: receipt $i" "$g" "$tmp/r" "$med" "$(jq -r ".final.receipts[$i].receiptId" "$f")"; then
				if [ "$(jq -r .predicate.accord "$tmp/statement")" = "$accord" ] && [ "$(jq -r .predicate.card "$tmp/statement")" = "$card" ]; then
					counted "$g"
				else bad "$n: receipt $i names another Accord or card"; fi
				chained "$n: receipt $i" "$g" "$tmp/chain"
				[ "$(jq -r .predicate.verdict "$tmp/statement")" = escalate ] && echo "sha256:$(sha "$tmp/statement")" >>"$tmp/escalates"
			fi
			i=$((i + 1))
		done
		# Every pending id is the re-computed id of its escalate receipt, the k-th open question the
		# k-th escalate written: no question is answered in this version (maybe-1).
		jq -r '.final.pending[]' "$f" >"$tmp/pending"
		if [ "$(wc -l <"$tmp/pending")" -ne "$(wc -l <"$tmp/escalates")" ]; then
			bad "$n: final.pending holds $(wc -l <"$tmp/pending" | tr -d ' ') ids for $(wc -l <"$tmp/escalates" | tr -d ' ') escalate receipts"
		else
			k=1
			while read -r id; do
				if [ "$id" = "$(sed -n "${k}p" "$tmp/escalates")" ]; then counted "$g"; echo "$g" >>"$tmp/pendings"; else bad "$n: pending id $k is not its escalate receipt's re-computed id"; fi
				k=$((k + 1))
			done <"$tmp/pending"
		fi
		ns=$(jq '.steps | length' "$f")
		i=0
		while [ "$i" -lt "$ns" ]; do
			s=".steps[$i]"
			case "$(jq -r "$s.op" "$f")" in
			give)
				if jq -e "$s.expect | has(\"receipt\")" "$f" >/dev/null; then
					jq -e "$s | has(\"messageHex\") and has(\"envelopeHex\")" "$f" >/dev/null ||
						{ bad "$n: step $i: an accepted give not given as messageHex and envelopeHex"; i=$((i + 1)); continue; }
					jq -r "$s.messageHex" "$f" | hex2bin >"$tmp/m"
					jq -r "$s.envelopeHex" "$f" | hex2bin >"$tmp/e"
					mh=sha256:$(sha "$tmp/m")
					jq . "$tmp/e" >"$tmp/env"
					if [ "$(jq -r .hash "$tmp/env")" = "$mh" ]; then counted "$g"; else bad "$n: step $i: the envelope's hash is not the message's"; fi
					caller=$(refof "$(jq -r ".keys[$s.as]" "$f")")
					sigcheck "$n: step $i: the envelope" "$g" "$tmp/env" "$tenv" "$caller"
					jq "$s.expect.receipt" "$f" >"$tmp/r"
					if receiptcheck "$n: step $i" "$g" "$tmp/r" "$med" "$(jq -r "$s.expect.receiptId" "$f")"; then
						if [ "$(jq -r .predicate.envelope.hash "$tmp/statement")" = "$mh" ]; then counted "$g"; else bad "$n: step $i: the receipt echoes another hash"; fi
						cp "$tmp/statement" "$tmp/st"
						jq '.parts[0]' "$tmp/m" >"$tmp/part"
						digestcheck "$n: step $i" "$g" "$tmp/part" "$tmp/st"
					fi
				fi
				;;
			get)
				if jq -e "$s.expect | has(\"delivery\")" "$f" >/dev/null; then
					jq "$s.expect.delivery" "$f" >"$tmp/d"
					jq .receipt "$tmp/d" >"$tmp/r"
					if receiptcheck "$n: step $i" "$g" "$tmp/r" "$med" "$(jq -r .receiptId "$tmp/d")"; then
						cp "$tmp/statement" "$tmp/st"
						digestcheck "$n: step $i" "$g" "$tmp/d" "$tmp/st"
					fi
				fi
				;;
			esac
			i=$((i + 1))
		done
		for who in $(jq -r '.final.delivered | keys[]' "$f"); do
			nd=$(jq --arg w "$who" '.final.delivered[$w] | length' "$f")
			i=0
			while [ "$i" -lt "$nd" ]; do
				jq --arg w "$who" ".final.delivered[\$w][$i]" "$f" >"$tmp/d"
				jq .receipt "$tmp/d" >"$tmp/r"
				if receiptcheck "$n: a delivery to $who" "$g" "$tmp/r" "$med" "$(jq -r .receiptId "$tmp/d")"; then
					cp "$tmp/statement" "$tmp/st"
					digestcheck "$n: a delivery to $who" "$g" "$tmp/d" "$tmp/st"
				fi
				i=$((i + 1))
			done
		done
		;;
	verify | reconcile)
		ref=$(jq -r .reference "$f")
		code=$(jq -r '.expect.code // ""' "$f")
		[ "$code" = reference ] && continue
		nr=$(jq '.receipts | length' "$f")
		upto=$nr
		case "$code" in "" | presented | kept) ;; *) upto=$(jq -r .expect.index "$f") ;; esac
		: >"$tmp/presented-ids"
		: >"$tmp/chain"
		i=0
		while [ "$i" -le "$upto" ] && [ "$i" -lt "$nr" ]; do
			bytesof "$f" ".receipts[$i]" receipt "$tmp/rb"
			if ! jq . "$tmp/rb" >"$tmp/r" 2>/dev/null; then
				[ "$i" -lt "$upto" ] && bad "$n: receipt $i before the failing one is not JSON"
				i=$((i + 1))
				continue
			fi
			if [ "$i" -lt "$upto" ]; then
				if receiptcheck "$n: receipt $i" "$g" "$tmp/r" "$ref"; then
					echo "sha256:$(sha "$tmp/statement")" >>"$tmp/presented-ids"
					chained "$n: receipt $i" "$g" "$tmp/chain"
				fi
			else
				case "$code" in
				statement-type | mediator | chain) receiptcheck "$n: receipt $i" "$g" "$tmp/r" "$ref" ;;
				# A receipt refused at row 7 passed row 6, its chain, so its prev is checked too.
				release) receiptcheck "$n: receipt $i" "$g" "$tmp/r" "$ref" && chained "$n: receipt $i" "$g" "$tmp/chain" ;;
				signature)
					if sound "$tmp/r" "$ref"; then
						jq -r '.payload' "$tmp/r" | base64 -d >"$tmp/statement"
						jq -r '.signatures[0].sig' "$tmp/r" | base64 -d >"$tmp/rsig"
						pae "$intoto" "$tmp/statement" "$tmp/rpae"
						if everify "$tmp/rpae" "$tmp/rsig" "$ref"; then bad "$n: receipt $i: openssl verifies a signature the vector refuses"; else counted "$g"; fi
					fi
					;;
				esac
			fi
			i=$((i + 1))
		done
		if [ "$op" = reconcile ] && [ -z "$code" ]; then
			for who in $(jq -r '.kept | keys[]' "$f"); do
				nd=$(jq --arg w "$who" '.kept[$w].delivered | length' "$f")
				: >"$tmp/kept-ids"
				i=0
				while [ "$i" -lt "$nd" ]; do
					bytesof "$f" ".kept[\"$who\"].delivered[$i]" receipt "$tmp/rb"
					jq . "$tmp/rb" >"$tmp/r"
					receiptcheck "$n: $who's kept receipt $i" "$g" "$tmp/r" "$ref" && echo "sha256:$(sha "$tmp/statement")" >>"$tmp/kept-ids"
					i=$((i + 1))
				done
				for id in $(jq -r --arg w "$who" '.expect.removed[] | select(.actor == $w) | .receiptId' "$f"); do
					if grep -qxF "$id" "$tmp/kept-ids"; then counted "$g"; else bad "$n: a removed id that is no kept receipt's of $who"; fi
				done
				for id in $(jq -r --arg w "$who" '.expect.unrecorded[] | select(.actor == $w) | .receiptId' "$f"); do
					if grep -qxF "$id" "$tmp/presented-ids"; then counted "$g"; else bad "$n: an unrecorded id that is no presented receipt's"; fi
				done
			done
		fi
		;;
	esac
done

for g in $groups; do
	c=$(grep -cxF "$g" "$tmp/counts")
	say "$g: $c hashes and signatures checked"
	[ "$c" -gt 0 ] || { say "FAIL: $g: nothing checked"; failed=1; }
done
# The prevs and pending ids, of all groups, re-computed as receipt ids above.
say "$(wc -l <"$tmp/prevs" | tr -d ' ') prevs and $(wc -l <"$tmp/pendings" | tr -d ' ') pending ids checked against re-computed receipt ids"
[ -s "$tmp/prevs" ] || { say "FAIL: no prev checked"; failed=1; }
[ -s "$tmp/pendings" ] || { say "FAIL: no pending id checked"; failed=1; }
if [ "$failed" -ne 0 ]; then say "refused: a hash or signature of the vectors does not hold under openssl (vectors/README.md)"; exit 1; fi
say ok
