# Custody: what GitHub must hold for the applier to be the only way to merge

Placket's receipt says the mediator released a part naming a commit. That the commit could not merge without it is GitHub's to hold, by the settings below. Follow them in order, for each repository the applier serves, before the agent gets any credential to it.

1. **Give the applier an identity of its own.** Create a GitHub App for it (Settings → Developer settings → GitHub Apps → New GitHub App) and install it on the repository alone (Install App → Only select repositories). Give it these repository permissions and no others: Contents, read and write, to merge; Pull requests, read and write, to read the pull request and comment on it; Commit statuses, read and write; Metadata, read. Where an App cannot be used, the fallback is a machine user, a GitHub account that is no person's, with a fine-grained token (Settings → Developer settings → Personal access tokens → Fine-grained tokens) limited to this repository and the same permissions. Never use a person's own token: the receipt names a decider, and the merge would then name the person too.

2. **Let only the applier merge into the base branch.** Add a branch ruleset (the repository's Settings → Rules → Rulesets → New ruleset → New branch ruleset):
   - Enforcement status: Active.
   - Target branches: the base branch the agent's pull requests merge into, such as Include default branch.
   - Bypass list: the applier's App, or its machine user, and nothing else: no role, no team, no other App, and not Repository admin.
   - Rules: Restrict updates, so that only the bypass list may update the branch, which is what a merge does; Restrict deletions; and Block force pushes.

   Then check it: with the agent's credential, try to merge a pull request into that branch, by the web page and by `gh pr merge`. Both must be refused. The agent's credential must not hold the Admin role on the repository or own its organisation, since either can edit the ruleset.

3. **Run the applier where the agent cannot reach it.** Run `apply.sh` on a machine or account the agent's processes cannot use, outside any repository or directory the agent can write to: an agent that can edit `apply.sh`, its `applied.jsonl` or its labels config decides what the applier merges. Keep the applier's credential, the App's private key or the machine user's token, and the applier's Placket key file where the agent cannot read them, such as that account's own secret store. Sign `gh` in there as the applier's identity (`gh auth login`, or `GH_TOKEN` in that account's environment only).

4. **Never rest custody on the status or a required check.** The `placket` commit status is written after the merge, for the auditor to find the receipt; it is not a gate. Do not make it, or any check, the thing that stops an agent's merge: the ruleset of step 2 does that. Anyone who may write statuses can set a status named `placket`, so an auditor trusts a merge only once `placket verify` checks the receipt it names (README.md, "The auditor's path").
