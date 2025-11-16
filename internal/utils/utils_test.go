package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormaliseCodeFences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic code fence normalization",
			input:    "Some text\n    ```go\n    fmt.Println(\"hello\")\n    ```",
			expected: "Some text\n```go\n    fmt.Println(\"hello\")\n```",
		},
		{
			name:     "diff markers inside code block",
			input:    "```diff\n    +func main() {\n    -func old() {\n```",
			expected: "```diff\n+func main() {\n-func old() {\n```",
		},
		{
			name:     "line ending with +``` should be split",
			input:    "Some code here+```\nMore text",
			expected: "Some code here\n+```\nMore text",
		},
		{
			name:     "line ending with -``` should be split",
			input:    "Some code here-```\nMore text",
			expected: "Some code here\n-```\nMore text",
		},
		{
			name:     "line ending with +``` and whitespace should be split",
			input:    "Some code here+```  \nMore text",
			expected: "Some code here\n+```\nMore text",
		},
		{
			name:     "code block with diff showing added code fence",
			input:    "```diff\n+```go\n+print(\"hello\")\n+```\n```",
			expected: "```diff\n+```go\n+print(\"hello\")\n+```\n```",
		},
		{
			name:     "multiple code blocks",
			input:    "First block:\n    ```js\n    console.log(\"test\")\n    ```\nSecond block:\n    ```py\n    print(\"test\")\n    ```",
			expected: "First block:\n```js\n    console.log(\"test\")\n```\nSecond block:\n```py\n    print(\"test\")\n```",
		},
		{
			name:     "diff with indented lines inside code block",
			input:    "```diff\n    +    if condition:\n    +        print(\"indented\")\n    -    if old:\n```",
			expected: "```diff\n+    if condition:\n+        print(\"indented\")\n-    if old:\n```",
		},
		{
			name:     "preserve non-diff lines inside code block",
			input:    "```go\n    func hello():\n        print(\"world\")\n```",
			expected: "```go\n    func hello():\n        print(\"world\")\n```",
		},
		{
			name:     "empty content",
			input:    "",
			expected: "",
		},
		{
			name:     "no code fences",
			input:    "Just some regular text\nwith multiple lines\nand no code blocks",
			expected: "Just some regular text\nwith multiple lines\nand no code blocks",
		},
		{
			name:     "code fence with language specifier ending with +```",
			input:    "text+```go",
			expected: "text\n+```go",
		},
		{
			name:     "realistic streaming scenario",
			input:    "Here's the fix:\n    ```go\n    +func NewFunction() {\n    +    return \"new\"\n    +}\ncontent+```",
			expected: "Here's the fix:\n```go\n+func NewFunction() {\n+    return \"new\"\n+}\ncontent\n+```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormaliseCodeFences(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormaliseCodeFences_PreservesIndentation(t *testing.T) {
	input := "```go\n    func function() {\n        if true {\n            print(\"deeply nested\")\n```"
	expected := "```go\n    func function() {\n        if true {\n            print(\"deeply nested\")\n```"

	result := NormaliseCodeFences(input)
	assert.Equal(t, expected, result)
}

func TestNormaliseCodeFences_DiffWithTabs(t *testing.T) {
	input := "```diff\n    +\tfunc() {\n    +\t\treturn\n    +\t}\n```"
	expected := "```diff\n+\tfunc() {\n+\t\treturn\n+\t}\n```"

	result := NormaliseCodeFences(input)
	assert.Equal(t, expected, result)
}
