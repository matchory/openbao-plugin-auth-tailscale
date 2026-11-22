# Documentation style

This guide is for anyone writing `README.md` or a page in `docs/`, whether you're a person or an agent. The pages
are read on GitHub.

## Voice

Write the way an experienced colleague explains something at a whiteboard: directly, precisely, and without padding.

- **Talk to the reader.** Use "you" and the present tense: "You give each container a hostname", not "The user
  configures a hostname". Contractions are fine.
- **Lead with what the reader wants.** Open a page or section with the outcome or the answer, then the details.
  Nobody needs "This page describes…" first.
- **Give the reason with the fact.** "Put `100.100.100.100` first: Docker asks only the first server and takes its
  answer" is easier to follow and to remember than the rule alone.
- **Be exact.** Name the command, value, default and condition. When something depends on a condition, state the
  condition instead of writing "usually" or "in most cases". When you quote a number, say where it was measured.
- **Be honest about limits.** Say plainly what the plugin doesn't do, what can go wrong, and whom the reader has to
  trust. A reader who is told the limits trusts the rest of the page.
- **Don't sell.** No "seamless", "robust", "powerful", "effortless" or "simply". If a feature is good, the facts
  will show it.

### Tells to avoid

Prose that reads generated loses the reader's trust. Cut these on sight:

- An opener that restates the heading ("In this guide, we will look at…").
- A closing paragraph that summarizes what the reader just read.
- Lists of three adjectives or three parallel clauses where one would do.
- Bold labels on every bullet when the bullets are short sentences anyway.
- "Note that", "It's worth mentioning", "Importantly".
- Linking words that only connect: "Thus", "Hence", "Additionally", "Furthermore".
- Hedges that stand in for a missing fact: "may", "might", "typically", "generally".

## Page types

Each page has one job.

| Type | Where | Its job |
| --- | --- | --- |
| Tutorial | `docs/getting-started.md` | Take a new user from nothing to a working login, one example, start to finish |
| Guide | `docs/guides/` | Help an operator do one task, including how to tell it worked |
| Reference | `docs/reference/` | List every fact about one interface, so the reader can look it up |
| Explanation | `docs/explanation/` | Say why the plugin works the way it does, and what follows from that |

A guide usually starts with what you'll have at the end and what you need first, then the steps, then how to
check the result and where to go next. Skip a section when it would be empty.

A reference page has a short introduction, then one entry per item (a field or a metadata key): its heading, a
table of facts (type, default), then what it does and its conditions, in prose or a short list. Reference pages
don't give advice; they link to the guide that does.

Every fact has one home. Other pages link to it instead of repeating it, unless the reader needs it right there to
follow a step.

## Terms

Use one term for one concept.

| Use | Meaning | Not |
| --- | --- | --- |
| caller | The program that sends the login request | client, workload |
| node | A device in the tailnet, as `tailscaled` reports it | machine, peer; device, except when quoting the admin console |
| tag | A Tailscale tag (`tag:<name>`) | label |
| role | A role of this auth method (`auth/tailscale/role/<name>`) | profile |
| source address | The address OpenBao sees a request come from | remote IP, client IP |
| tailnet address | An address Tailscale assigned to a node | Tailscale IP |
| OpenBao node | One server of an OpenBao cluster | voter; *node* alone, which means a tailnet node |
| loopback listener | An OpenBao listener bound to `127.0.0.1` behind `tailscale serve` | local listener |
| Service | A Tailscale Service (`svc:<name>`) | VIP service; *service* in lowercase |
| refuse | What the plugin does to a login it does not accept | reject, deny, block |

## Markdown

- One H1 per file.
- Lines of at most 120 characters. Tables are exempt.
- `-` for bullets; number ordered lists `1.`, `2.`, `3.`.
- Give every fenced code block a language: `bash`, `hcl`, `json`, `text` or `mermaid`.
- Link between pages with relative `.md` links, anchors included: `[roles](../reference/roles.md#bound_tags)`.
- Draw diagrams in Mermaid. GitHub renders them.
- Use `> [!WARNING]` only for a risk to data, security or availability, and `> [!NOTE]` for anything else worth
  pulling out. Use them sparingly: a page full of callouts has none.
- Put commands, fields, paths and values in code format.
- Don't write dates or provisional text such as "still to be tested". History belongs in `CHANGELOG.md`.

`SECURITY.md` holds the security guarantees and the environment properties they assume. A test reads its
`### G<n>:` headings, so don't change their format.

## Before and after

| Before | After |
| --- | --- |
| This page describes how the plugin decides whether a login is accepted. | A login succeeds when `tailscaled` says the request's source address belongs to a node that carries the role's tags. Here's each check, in order. |
| The listener must only trust loopback. Thus other addresses are ignored. | Trust forwarded addresses from `127.0.0.1` only: any other sender could name an address that isn't its own. |
| In most cases, tokens should be short-lived. | Give roles a short `token_ttl`: logging in again costs the caller one request and no secret. |
