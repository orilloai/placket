# The notifier

Each new question runs one command of yours, as it opens, with its label and its id: a desktop notification, a message, a line in a log. The person who answers hears of a question when it opens, instead of asking for the open questions again and again.

## The question

"How do I hear of a question the moment it is asked of me, without asking placket every few seconds?"

## When to use it

Use it when a person answers the questions of one or more mediators, and wants to be told of each as it opens. `placket watch` waits on each label's questions at the door, holding no lock, and prints each new question once; the pattern runs your command for each. The person then answers with the `placket answer`, `change` or `decline` line `watch` printed.

It is not the pattern for answering: `watch` never answers, and neither does the pattern. Nor is it for an agent that asks questions: an agent reaches placket through `placket mcp`, whose `give` returns the question's id and whose `result` waits for the closing.

## What it relies on

From `spec/protocol.md`:

- §6, Questions, the three answers and release: a question opens when the terms ask about a part, and its answerers are shown it while it is open.
- §11, What the command line prints for a script: `mediators`, `give`, `watch` and `answer`, with `--json`.

And the mediator API's `questions` call with `wait` and `after` (`docs/reference/mediator-api.md`), on which `watch` is built.

## What it claims

The pattern claims nothing a receipt does not. A notification says a question opened; the ask's receipt is what says the mediator held the part and asked. The pattern leans on the claim line `show` prints for an ask, and on the scope line that follows every claim line:

```
claim: the mediator held this part and asked the roles its terms name. This receipt delivered nothing.
not claimed: what any party did with a part after it crossed; anything that did not go through this mediator; reworded or derived content; who was at a key.
```

## Making it yours

- `notify` is the command each new question runs, with its label and id. As it ships it prints one line. On macOS, `osascript -e "display notification \"$2\" with title \"placket: $1\""` shows a notification; on Linux, `notify-send "placket: $1" "$2"`.
- `config/placket/mediators.json` is the labels config: each entry an Accord id, a key file, a founding set and, if it is not the Accord's default, a socket, with a label. Your own lives in `$XDG_CONFIG_HOME/placket/` (else `~/.config/placket/`); the pattern keeps its own in its directory.
- `--count 2` stops `watch` after two questions, so the pattern ends. Leave it out and `watch` runs until it is stopped. After a restart it prints the questions still open again.
