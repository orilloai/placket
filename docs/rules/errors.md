# Errors

How a failure is typed, coded and worded, in the core and in the adapters. Errors are codes, never quotes of the input.

## `one-error-type`

Each package has one error type, `*Error`, whose `Code` is a constant of a stable string type, spelled as the requirement text spells it.

Why: callers and vectors match on codes, never on text.

Checked by: the reviewer.

## `errors-as-codes`

When input is refused, the caller gets a code, not a decoder's error wrapped up; the one wrap allowed is another placket package's `*Error`, found with `errors.As`.

Why: a decoder's message quotes its input, and the input may be secret.

Checked by: the reviewer.

## `rejection-vs-error`

What the caller did wrong (a protocol rejection) and what the caller's program did wrong (a tool error) are different types, and a door's own code is a third, never converted into either. Each new failure is placed in one, and the design says which.

Why: the two go to different people and are recorded differently.

Checked by: the reviewer.

## `error-text-no-content`

Error text is the package and the code, plus an index, an actor or an offset where the requirement gives one; never a body, salt, seed, key, message or payload. Each error type has a table test that plants those and asserts they are absent.

Why: errors end up in logs.

Checked by: the reviewer.

## `sentinels-uncoded`

A sentinel error is only for a failure that is not a protocol refusal and carries no code, and is matched with `errors.Is`.

Why: a coded failure matched by identity loses its code.

Checked by: the reviewer.

## `check-order`

Where the requirement text fixes the order of checks, the code keeps it, and a test feeds an input that fails two of them at once. Where it is silent, the design records the order chosen.

Why: which code comes back is part of the API.

Checked by: the reviewer.
