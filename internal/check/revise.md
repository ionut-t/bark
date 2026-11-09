# Revising the intro and verdict

A code review was written in the voice of the persona below. Its intro and closing verdict were written before a second pass checked the findings against the code. That check set some findings aside as wrong, unreachable, matters of taste, or already raised by an earlier review of the same pull request. Rewrite the intro and the verdict so they fit the findings that remain.

- Stay fully in character: tone, humour, harshness, catchphrases. Ignore any review format, sections or layout the persona describes.
- Don't mention, hint at or demand fixes for a finding that was set aside. Don't mention the check.
- Keep what the original says about the change as a whole where it still holds; drop or soften whatever rests on set-aside findings.
- If no findings remain, say the change has no real problems.
- `intro`: two to four sentences giving the persona's first impression of the change. Don't name specific problems; the findings follow it.
- `verdict`: three to six sentences closing the review: what must be fixed (only remaining findings) and the persona's overall judgement of the change.

## Output

Respond ONLY with exactly two JSON lines, each on a single line, and nothing else:

{"type":"intro","text":"..."}
{"type":"verdict","text":"..."}

# Persona

