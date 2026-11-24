# 0015. No payload signature under a placket URI

Status: accepted. Date: 2026-11-24. Changes records 0002 and 0005.

## Context

Signing-4 keeps a payload signature from standing for a placket object's signature: a payload type is never one of the five type URIs under which placket signs objects. In placket v0.1.0, `signing.SignPayload` and `signing.VerifyPayload` held that rule by refusing exactly those five strings, and signed and verified any other payload type.

So a key that signs payloads would sign under any other URI of placket's host, `https://placket.tech/`: a kind no version signs yet, such as a registration, or one of the five kinds under a later protocol number, such as `https://placket.tech/declaration/0.2`. A later version that signs objects under such a URI would then meet payload signatures a 0.1 core had made under it, by the same keys, over the same pre-authentication encoding. A signature made today as a payload could be shown to that version as an object's signature.

No payload that protocol 0.1 signs or verifies has a payload type under placket's host. A receipt and a close receipt are signed under `application/vnd.in-toto+json` (receipt-1, close-2), a door's call under `application/vnd.placket.call+json`, and a verifier refuses any other receipt payload type before it checks a signature (section 8).

## Decision

- **A payload signature is never made or checked under a payload type that starts with `https://placket.tech/`.** `SignPayload` refuses such a payload type with the code it already returns for the five, `type-uri` (`signing.CodeTypeURI`), and `VerifyPayload` reports false for it. The five type URIs are among them.
- **The match is byte-exact.** The prefix is the 21 bytes `https://placket.tech/`. Another scheme, another case, or the host with no trailing slash is not placket's URI and signs as any other payload type: a URI of placket's host is always written in that one spelling (record 0005), so a later version's type URI starts with those bytes.
- **Objects keep their five type URIs.** `Sign`, `Verify` and `SigningBytes` sign and verify objects under exactly the five type URIs of signing-4, and under no other URI of placket's host. The prefix check is for payload types only.
- **The prefix is an unexported constant of `core/signing`.** No exported name or code is added.
- **The requirement text is unchanged in protocol 0.1.** Signing-4 forbids the five type URIs as payload types; refusing more refuses nothing that 0.1 signs or verifies, so the core stays conformant to signing-4 as written. Signing-4's text takes the wider rule with the next protocol version.
- **It ships in the core module from `core/v0.1.1`.** The root module chooses no payload type under placket's host, so the binary behaves the same either side of the tag.

This changes record 0002, whose list of the core's names says `SignPayload` and `VerifyPayload` sign under any payload type but placket's type URIs: they now sign under any payload type outside `https://placket.tech/`. It changes record 0005, under which the five type URIs were the only placket URIs a payload signature refused: every URI under the one host is now refused as a payload type.

## Consequences

- A Go project that signs payloads with the core from `core/v0.1.1` can never make a signature under a URI that a later protocol version may use for an object.
- A caller of `core/v0.1.0` that signed a payload under some other URI of placket's host gets `type-uri` from `core/v0.1.1`, and a signature it made earlier no longer verifies. No placket format uses such a payload type.
- A later protocol version that adds an object type adds its URI to the object types alone; payload signatures already refuse it.
