# Checking review findings

Another reviewer read a code change and reported the findings listed below. Automated reviewers often report problems that the surrounding code already handles, that can't happen, or that are only preferences, and also find real defects. Your job is to check each finding against the code and keep the real defects.

You get the change as a diff (every added and context line starts with its line number in the new file), the full new version of each file the findings point to, and the findings, numbered from 1.

For each finding, work through these questions, using the code you were given rather than memory or assumptions:

1. **Does the claim match the code?** Find the lines it is about. Read the surrounding code in the full file, not just the diff: the problem may already be handled a few lines away, by the caller, or by the type of a value.
2. **Is the claim about the language or its standard library true for the version in use?** If the context says which Go version the module targets, functions and methods added up to that version exist. Don't drop a finding because you don't recognise an API, and don't keep one that claims an API is missing, misbehaves or allocates unless you are certain of it.
3. **Can it actually happen?** Name the concrete input or state that makes it fail, and check that this input can reach the code in that shape. Input produced by the program's own code or by a well-known tool (such as git or the GitHub CLI) follows that tool's format, not every format imaginable; input from users, files, the network or a language model can be anything. A failure that needs input the code never receives, or that only matters at sizes the code never sees, isn't a defect.
4. **Is it a defect or a preference?** Style, structure, "over-engineered", "use library X instead", naming, receiver kinds, adding context to an error that already has it, or an error that is ignored on purpose with no effect on the user: these are preferences. Drop them.
5. **If it has a suggestion, would the suggestion compile and work where it goes?** Every name it uses must be in scope at that line. It must not break the code around it.

Keep a finding if it describes a real defect: a realistic input or state on which the code produces a wrong result, loses data, crashes or misleads the user. Calling the code a heuristic, best effort or a deliberate trade-off doesn't excuse a wrong result that is then used; check where the result goes. The finding's explanation or suggested fix may be off while the problem it points at is real: judge the problem. Drop a finding when the code shows the claim is wrong, the failure can't happen, or it is only a preference.

## Output

Respond ONLY with JSON Lines: one object per finding, in order, each on a single line. No Markdown, no code fences, no other text.

{"id":1,"verdict":"keep","severity":"high","suggestion":"ok","evidence":"...","reason":"..."}

- `id`: the finding's number.
- `verdict`: `keep` or `drop`.
- `evidence`: the line or lines of code your verdict rests on, copied exactly from the diff or the files (without line numbers or diff markers; separate lines with `\n`). Required when you keep a finding: a finding you can't back with code you were given must be dropped.
- `reason`: one or two plain sentences: the concrete input or state on which the code goes wrong and what happens to the wrong result, or why the finding is wrong, unreachable or a preference.
- `severity`: for a kept finding, one of `critical`, `high`, `medium`, `low`, judged again from what you found. Omit it for a dropped finding.
- `suggestion`: for a kept finding that has a suggestion, `ok` if it compiles and fixes the problem in place, otherwise `broken`. Omit it otherwise.
