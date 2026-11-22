# Faults

A [file that is not there](nofile.md), a [heading that is not there](approve.md#no-such-heading), and a [heading that is](approve.md#2-start-the-window), with `[a span](nofile.md)` that is code.

```sh
placket nosuchcommand --json
placket give --no-such-flag x
placket init nosuchdoor
placket init approve --dir "$HOME/x" 2>init.out
placket key new --out a.key && placket key ref --key a.key
placket verify --receipts journal --accord ID --founding founding.json \
  --no-such-flag
```

[defined]: nofile2.md
