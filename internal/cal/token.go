package cal

import "strings"

type TokenKind int

const (
	TokEOF TokenKind = iota
	TokIdent
	TokQuotedIdent
	TokNumber
	TokString
	TokWhitespace
	TokNewline
	TokLineComment
	TokBlockComment

	TokAssign      // :=
	TokPlusAssign  // +=
	TokMinusAssign // -=
	TokMulAssign   // *=
	TokDivAssign   // /=
	TokNE          // <>
	TokLE          // <=
	TokGE          // >=
	TokEQ          // =
	TokLT          // <
	TokGT          // >
	TokPlus        // +
	TokMinus       // -
	TokStar        // *
	TokSlash       // /
	TokRange       // ..
	TokScope       // ::
	TokDot         // .
	TokComma       // ,
	TokSemi        // ;
	TokColon       // :
	TokAt          // @
	TokLParen      // (
	TokRParen      // )
	TokLBracket    // [
	TokRBracket    // ]
	TokOther
)

// Token represents a lossless lexical token in C/AL source code.
type Token struct {
	Kind  TokenKind
	Text  string
	Start int
	End   int
	Line  int
	Col   int
}

// IsTrivia reports whether the token is whitespace, newline, or comment.
func (t Token) IsTrivia() bool {
	return t.Kind == TokWhitespace || t.Kind == TokNewline || t.Kind == TokLineComment || t.Kind == TokBlockComment
}

// IsComment reports whether the token is a // or { } comment.
func (t Token) IsComment() bool {
	return t.Kind == TokLineComment || t.Kind == TokBlockComment
}

// IsWord reports whether the token is an unquoted identifier matching word case-insensitively.
func (t Token) IsWord(word string) bool {
	return t.Kind == TokIdent && strings.EqualFold(t.Text, word)
}

// IdentName returns the unquoted name for TokIdent or TokQuotedIdent.
func (t Token) IdentName() string {
	if t.Kind == TokQuotedIdent && len(t.Text) >= 2 {
		inner := t.Text[1 : len(t.Text)-1]
		return strings.ReplaceAll(inner, `""`, `"`)
	}
	return t.Text
}

// Render concatenates token texts losslessly.
func Render(tokens []Token) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteString(t.Text)
	}
	return b.String()
}

// NextCode returns the index of the next non-trivia token at or after idx, or -1.
func NextCode(tokens []Token, idx int) int {
	for i := idx; i < len(tokens); i++ {
		if !tokens[i].IsTrivia() && tokens[i].Kind != TokEOF {
			return i
		}
	}
	return -1
}

// PrevCode returns the index of the previous non-trivia token at or before idx, or -1.
func PrevCode(tokens []Token, idx int) int {
	if idx >= len(tokens) {
		idx = len(tokens) - 1
	}
	for i := idx; i >= 0; i-- {
		if !tokens[i].IsTrivia() && tokens[i].Kind != TokEOF {
			return i
		}
	}
	return -1
}

// FormatIdent formats an identifier name for C/AL (adds quotes if needed).
func FormatIdent(name string, forceQuotes bool) string {
	if forceQuotes || NeedsQuotes(name) {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return name
}

// NeedsQuotes reports whether an identifier requires double quotes in C/AL.
func NeedsQuotes(name string) bool {
	if name == "" {
		return true
	}
	if IsStatementKeyword(name) {
		return true
	}
	for i, r := range name {
		if i == 0 {
			if !isIdentStartRune(r) {
				return true
			}
		} else {
			if !isIdentPartRune(r) {
				return true
			}
		}
	}
	return false
}
