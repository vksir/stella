# Domain Docs

How engineering skills should consume this repo's domain documentation.

## Before exploring, read these

- `CONTEXT.md` at the repo root.
- Relevant ADRs in `docs/adr/`.

If any of these files do not exist, proceed silently. Do not flag their absence or suggest creating them upfront. Create them only when domain terms or decisions are actually resolved.

## File structure

This is a single-context repo:

- `CONTEXT.md` contains shared domain terminology.
- `docs/adr/` contains architecture decision records.

## Use the glossary's vocabulary

When naming a domain concept in an issue, proposal, or test, use the term defined in `CONTEXT.md`. If the needed concept is missing, note the gap rather than inventing a competing term.

## Flag ADR conflicts

If a proposal contradicts an existing ADR, surface the conflict explicitly rather than silently overriding it.
