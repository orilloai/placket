# Quickstart: an out-gate on a merge

An agent proposes to merge a pull request on GitHub, you accept or decline it, and an applier merges only what you accepted, at the head commit you accepted. It uses Claude Code as the agent, and needs `placket` on your `PATH` ([install](../../README.md#install)), `gh` and `jq`, and no judge's key; [`patterns/github-merge/`](../../patterns/github-merge/README.md) rehearses it against a stand-in `gh`, with no repository. Without Claude Code, that rehearsal is one of [two results that need no agent](../../README.md#with-no-agent).

The words it uses, Accord, terms, ask, answer, receipt and verify, are in [the approve guide's "Six words"](../guides/approve.md#six-words).

```sh
placket init merge
```

`init` founds the example Accord for you alone: the agent, you in the role `decider`, and the applier that merges, each with its own key, all in `~/.config/placket/merge/`. It prints the Accord's id, the three parties, and every command that comes next:

1. Set up the repository first, as [the custody checklist](../../patterns/github-merge/custody.md) says: until the ruleset lets only the applier merge, the flow protects nothing.
2. Start the window with the `placket window` line it printed, and leave it running.
3. Add placket to Claude Code with the `claude mcp add` line, and ask the agent to propose a merge: the repository, the pull request, its base and its head commit.
4. Watch for the question, check the pull request at that head, and answer it:

   ```sh
   placket watch --label merge-person
   ```

5. Fetch the applier and run it: it merges only a release you accepted, and records it on the pull request.

[The guide](../guides/merge.md) takes each step in full, from `init` to a merge commit anyone can follow back to its receipt.
