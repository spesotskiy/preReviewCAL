package cal

import (
	"unicode"
	"unicode/utf8"
)

// Lex tokenizes C/AL source code into a lossless stream of Tokens (including trivia).
func Lex(src string) []Token {
	var tokens []Token
	i := 0
	line := 1
	col := 1

	advanceLineCol := func(text string) {
		for _, r := range text {
			if r == '\n' {
				line++
				col = 1
			} else if r != '\r' {
				col++
			}
		}
	}

	emit := func(kind TokenKind, start, end int) {
		text := src[start:end]
		tokens = append(tokens, Token{
			Kind:  kind,
			Text:  text,
			Start: start,
			End:   end,
			Line:  line,
			Col:   col,
		})
		advanceLineCol(text)
	}

	for i < len(src) {
		ch := src[i]

		// Newline (\r\n or \n)
		if ch == '\r' && i+1 < len(src) && src[i+1] == '\n' {
			emit(TokNewline, i, i+2)
			i += 2
			continue
		}
		if ch == '\n' {
			emit(TokNewline, i, i+1)
			i++
			continue
		}

		// Horizontal whitespace
		if ch == ' ' || ch == '\t' {
			start := i
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
			emit(TokWhitespace, start, i)
			continue
		}

		// Line comment //
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			start := i
			i += 2
			for i < len(src) && src[i] != '\n' && src[i] != '\r' {
				i++
			}
			emit(TokLineComment, start, i)
			continue
		}

		// Block comment { ... } (supports nested braces in C/AL)
		if ch == '{' {
			start := i
			depth := 1
			i++
			for i < len(src) && depth > 0 {
				if src[i] == '{' {
					depth++
				} else if src[i] == '}' {
					depth--
				}
				i++
			}
			emit(TokBlockComment, start, i)
			continue
		}

		// String literal '...' (doubled '' escapes quote)
		if ch == '\'' {
			start := i
			i++
			for i < len(src) {
				if src[i] == '\'' {
					if i+1 < len(src) && src[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				if src[i] == '\n' {
					break
				}
				i++
			}
			emit(TokString, start, i)
			continue
		}

		// Quoted identifier "..." (doubled "" escapes quote)
		if ch == '"' {
			start := i
			i++
			for i < len(src) {
				if src[i] == '"' {
					if i+1 < len(src) && src[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				if src[i] == '\n' {
					break
				}
				i++
			}
			emit(TokQuotedIdent, start, i)
			continue
		}

		// Number / Date / Time / DateTime literal (e.g. 123, 12.34, 0D, 010126D, 120000T, 0DT, 100L)
		if ch >= '0' && ch <= '9' {
			start := i
			for i < len(src) && src[i] >= '0' && src[i] <= '9' {
				i++
			}
			// Decimal part (avoid consuming '..' range operator!)
			if i < len(src) && src[i] == '.' && (i+1 >= len(src) || src[i+1] != '.') && (i+1 < len(src) && src[i+1] >= '0' && src[i+1] <= '9') {
				i++
				for i < len(src) && src[i] >= '0' && src[i] <= '9' {
					i++
				}
			}
			// Optional C/AL literal suffix: DT, D, T, L
			if i+1 < len(src) && (src[i] == 'D' || src[i] == 'd') && (src[i+1] == 'T' || src[i+1] == 't') && !isIdentPartByte(src, i+2) {
				i += 2
			} else if i < len(src) && (src[i] == 'D' || src[i] == 'd' || src[i] == 'T' || src[i] == 't' || src[i] == 'L' || src[i] == 'l') && !isIdentPartByte(src, i+1) {
				i++
			}
			emit(TokNumber, start, i)
			continue
		}

		// Identifier (Unicode-aware for Russian/non-ASCII identifiers if any)
		r, size := utf8.DecodeRuneInString(src[i:])
		if isIdentStartRune(r) {
			start := i
			i += size
			for i < len(src) {
				r2, sz2 := utf8.DecodeRuneInString(src[i:])
				if isIdentPartRune(r2) {
					i += sz2
				} else {
					break
				}
			}
			emit(TokIdent, start, i)
			continue
		}

		// Two-char operators
		if i+1 < len(src) {
			two := src[i : i+2]
			var kind TokenKind
			switch two {
			case ":=":
				kind = TokAssign
			case "+=":
				kind = TokPlusAssign
			case "-=":
				kind = TokMinusAssign
			case "*=":
				kind = TokMulAssign
			case "/=":
				kind = TokDivAssign
			case "<>":
				kind = TokNE
			case "<=":
				kind = TokLE
			case ">=":
				kind = TokGE
			case "..":
				kind = TokRange
			case "::":
				kind = TokScope
			}
			if kind != 0 {
				emit(kind, i, i+2)
				i += 2
				continue
			}
		}

		// Single-char operators & punctuation
		var kind TokenKind
		switch ch {
		case '=':
			kind = TokEQ
		case '<':
			kind = TokLT
		case '>':
			kind = TokGT
		case '+':
			kind = TokPlus
		case '-':
			kind = TokMinus
		case '*':
			kind = TokStar
		case '/':
			kind = TokSlash
		case '.':
			kind = TokDot
		case ',':
			kind = TokComma
		case ';':
			kind = TokSemi
		case ':':
			kind = TokColon
		case '@':
			kind = TokAt
		case '(':
			kind = TokLParen
		case ')':
			kind = TokRParen
		case '[':
			kind = TokLBracket
		case ']':
			kind = TokRBracket
		default:
			kind = TokOther
		}
		emit(kind, i, i+size)
		i += size
	}

	return tokens
}

func isIdentStartRune(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPartRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// IsIdentPartRune reports whether r may appear inside an unquoted C/AL identifier
// (letter, digit or underscore).
func IsIdentPartRune(r rune) bool {
	return isIdentPartRune(r)
}

func isIdentPartByte(s string, idx int) bool {
	if idx >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[idx:])
	return isIdentPartRune(r)
}
