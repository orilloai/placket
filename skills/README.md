# Skills

Two skills for an agent that helps a person try placket. Each is a directory holding a `SKILL.md`, with `name` and `description` front matter and plain instructions, and no syntax of one agent's own, so any agent that reads skills can use it.

- `placket-setup`: checks that `placket` is installed, and with the person's yes installs it. Then it sets up one Accord with them: from a pattern of the release their `placket version` names, which `fetch-pattern.sh` copies from the repository at that release's tag, or as an Accord of their own, whose terms it drafts and shows before anything is signed. It ends with the line that adds `placket mcp` to their agent.
- `placket-scout`: reads the person's repository or product description and lists where an agent's action cannot be undone or leaves their system, each with the pattern or kind of term that would gate it and what a receipt there would prove. It reads only locally, sends nothing and changes nothing, and offers `placket-setup` for any one of them.

Neither skill answers a question placket asks, or accepts a view without asking the person.

## Adding them to an agent

- **Claude Code**: the plugin at the repository's root bundles both. `/plugin marketplace add orilloai/placket`, then `/plugin install placket@placket`. The plugin has no version of its own, so it follows the repository's `main`.
- **Any other agent**: copy a skill's directory, `skills/placket-setup/` or `skills/placket-scout/`, whole, to where that agent reads skills. `placket-setup` needs its `fetch-pattern.sh` beside its `SKILL.md`.

The skills follow `main`. Where a skill names a command the installed `placket` does not have, it says to upgrade.

`tests/test_skills.sh`, in `make ci`, checks each `SKILL.md`'s front matter, that every `placket` command it names is one the binary lists, `fetch-pattern.sh`, and the plugin's manifests.
