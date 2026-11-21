# What placket does not do

Placket mediates what crosses a handoff and signs a receipt for each decision. Its claims stop there. Each line below is a limit, stated as one; [the threat model](../security/threat-model.md) gives the full list, with the reasons.

- **It says nothing of what a receiver does with a part once it has it.** A receipt records that a part crossed, or did not; what any party did with it after is no claim of placket's. Every claim line is followed by the scope line that says so.
- **It runs no command.** Placket passes parts and holds them. What happens at the edges is the parties' own: an applier, a script you write, acts on what was released ([writing an applier](../guides/appliers.md)).
- **It does not stop an action that does not go through it.** That an agent cannot merge, write or send some other way rests on the agent's credentials, accounts and the forge's settings, which placket neither sets nor checks ([custody](../guides/custody.md)).
- **It does not show who was at a key.** A receipt names keys. Whoever holds a key file can act as its party.
- **It does not see reworded or derived content.** A receipt names one part by its digest; a part reworded, summarised or split is another part.
- **It does not show that the mediator is honest, or that its clock is true.** In this version the window is run by one party to the Accord, who holds the mediator's key and every party's terms. The other parties check the journal against their own kept records ([the mediator](mediator.md)).
- **It does not claim a journal is complete without a close.** A journal cut short after any receipt still verifies; completeness is claimed only against the close receipt, and never that the mediator's key signed no other journal ([verify](../verify.md)).
- **It does not make a judge right.** A judge's pass is a model's, and text inside the part can steer it. The terms bound where a judge may decide, and a judge that fails leaves the part to a person ([judges](../guides/judges.md)).
- **It holds no key but the ones you make or bring.** Each key is a file of yours, and a judge's key is your own, read once from `PLACKET_JEV_KEY` and written nowhere.
