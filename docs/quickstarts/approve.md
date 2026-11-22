# Quickstart: approve before apply

An agent proposes the text of a file, you accept, change or decline it, and an applier writes only what you accepted. This is the entry point to try first. It needs `placket` on your `PATH` ([install](../../README.md#install)) and no judge's key.

```sh
placket init approve
```

`init` founds the example Accord for you alone: the agent, you in the role `decider`, and the applier, each with its own key, all in `~/.config/placket/approve/`. It prints the Accord's id, the three parties, and every command that comes next:

1. Start the window with the `placket window` line it printed, and leave it running.
2. Add placket to Claude Code with the `claude mcp add` line, and ask the agent to propose a file's text.
3. Watch for the question, read it, and answer it:

   ```sh
   placket watch --label approve-person
   placket questions --label approve-person
   ```

4. Fetch the applier and run it: it writes the text you accepted, or your version.
5. Verify the journal with the `placket verify` line of the guide.

As `init` sets it up, every key is in your own account, so the gate does not hold against an agent that can run commands as you. [The guide](../guides/approve.md) says why, and takes each step in full, from `init` to a verified receipt.
