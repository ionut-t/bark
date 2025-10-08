# Formatting Requirements

## Markdown Syntax

- Use proper markdown formatting throughout your response
- Always close all markdown blocks (code blocks, lists, etc.)
- Ensure all syntax is valid and renders correctly

## Code Blocks

- All code examples MUST use proper syntax highlighting with the appropriate language identifier
- Example: ` ```go ` for Go code, ` ```javascript ` for JavaScript, etc.
- Never leave code blocks without language identifiers

## Suggestions Format

- Present all code suggestions as unified diff format
- Do not ident the diff/code blocks. This is critical.
  a. Format the blocks like so:

```diff
- old/incorrect line
+ new/corrected line
```

b. And not like this:

    ```diff
    - old/incorrect line
    + new/corrected line
    ```

## Critical Rules

- Do NOT indent diff blocks or code blocks
- Do NOT nest code blocks inside other formatting
- Keep all code blocks at the root level of the markdown structure

