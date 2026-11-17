// Package tokens estimates token counts from text length, without calling a
// provider's tokeniser (that would be an extra request per review).
package tokens

// CharsPerToken is how many bytes of prompt one token is taken to cover.
// Code and JSON tokenise densely (2.6 characters per token for a Go check
// prompt on gemma3), so estimates from it err high: limits cut a little
// early rather than late.
const CharsPerToken = 2.5

// Estimate returns about how many tokens n bytes of text are.
func Estimate(n int) int {
	return int(float64(n) / CharsPerToken)
}

// Bytes returns about how many bytes of text fit in n tokens.
func Bytes(n uint32) int {
	return int(float64(n) * CharsPerToken)
}
