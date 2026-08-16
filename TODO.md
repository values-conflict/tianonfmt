<!-- remove from file when complete; keep a double space between TODO entries so they're more readable / digestible -->
<!-- sub-bullets (2-space indent, `-`, cuddled -- no blank line between parent and sub-bullets, nor between sibling sub-bullets) are for related side notes subordinate to the main item but distinct enough to stand alone -- use a semicolon continuation for the same thought, a sub-bullet for a related angle, and a new top-level entry for a separate concern -->

- `// TODO: ...` should be auto-converted to `// TODO ...` (no colon)

- godoc comment wrapping -- for exported doc comments, we should figure out / consider sentence wrapping (https://github.com/jbeda/mdreflow), and for inline struct comments we should hard-wrap/reflow at 80 columns (since they render verbatim in the godoc web view)
  - maybe for *all* markdown we should lean into sentence-per-line too?

- would it be too much scope creep to add a minifier mode?  (using our AST parsers to then remove anything that's "fluff")
  - we could then use this to find edge cases in how we choose to render things
  - however often our choices are based on "leave it how it was" so that's not exactly perfect
