package xpr

import "fmt"

type tokenType int

const (
	tokNumber tokenType = iota
	tokString
	tokBoolean
	tokNull
	tokTemplateLiteral
	tokTemplateHead
	tokTemplateMiddle
	tokTemplateTail
	tokIdentifier
	tokPlus
	tokMinus
	tokStar
	tokSlash
	tokPercent
	tokStarStar
	tokEqualEqual
	tokBangEqual
	tokLess
	tokGreater
	tokLessEqual
	tokGreaterEqual
	tokAmpAmp
	tokPipePipe
	tokBang
	tokQuestionQuestion
	tokQuestionDot
	tokPipeGreater
	tokArrow
	tokQuestion
	tokColon
	tokComma
	tokDot
	tokLeftParen
	tokRightParen
	tokLeftBracket
	tokRightBracket
	tokLeftBrace
	tokRightBrace
	tokSemicolon
	tokDotDotDot
	tokLet
	tokEqual
	tokRegex
	tokEOF
)

type token struct {
	typ      tokenType
	value    string
	position int
}

func processEscape(ch byte) string {
	switch ch {
	case 'n':
		return "\n"
	case 't':
		return "\t"
	case 'r':
		return "\r"
	case '0':
		return "\x00"
	case '\\':
		return "\\"
	case '\'':
		return "'"
	case '"':
		return "\""
	default:
		return string(ch)
	}
}

var regexAfterTokens = map[tokenType]bool{
	tokEqual: true, tokEqualEqual: true, tokBangEqual: true,
	tokLeftParen: true, tokLeftBracket: true, tokLeftBrace: true,
	tokComma: true, tokSemicolon: true, tokQuestion: true, tokColon: true,
	tokPipeGreater: true, tokArrow: true, tokAmpAmp: true, tokPipePipe: true,
	tokQuestionQuestion: true, tokBang: true, tokPlus: true, tokMinus: true,
	tokStar: true, tokSlash: true, tokPercent: true, tokStarStar: true,
	tokLess: true, tokGreater: true, tokLessEqual: true, tokGreaterEqual: true,
	tokDotDotDot: true, tokLet: true,
}

func tokenize(src string) ([]token, error) {
	tokens := []token{}
	pos := 0
	n := len(src)
	var lastType tokenType = -1

	peek := func(offset int) byte {
		idx := pos + offset
		if idx < n {
			return src[idx]
		}
		return 0
	}

	advance := func() byte {
		if pos < n {
			ch := src[pos]
			pos++
			return ch
		}
		return 0
	}

	readString := func(quote byte, start int) (token, error) {
		val := []byte{}
		for pos < n {
			ch := advance()
			if ch == quote {
				return token{tokString, string(val), start}, nil
			}
			if ch == '\n' {
				return token{}, fmt.Errorf("unterminated string at position %d", start)
			}
			if ch == '\\' {
				esc := advance()
				val = append(val, processEscape(esc)...)
			} else {
				val = append(val, ch)
			}
		}
		return token{}, fmt.Errorf("unterminated string at position %d", start)
	}

	readTemplateContent := func() (content string, ended bool, interpolation bool, err error) {
		buf := []byte{}
		for pos < n {
			ch := peek(0)
			if ch == '`' {
				advance()
				return string(buf), true, false, nil
			}
			if ch == '$' && peek(1) == '{' {
				advance()
				advance()
				return string(buf), false, true, nil
			}
			if ch == '\\' {
				advance()
				nxt := peek(0)
				if nxt == '$' || nxt == '`' || nxt == '\\' {
					buf = append(buf, advance())
				} else {
					buf = append(buf, processEscape(advance())...)
				}
			} else {
				buf = append(buf, advance())
			}
		}
		return "", false, false, fmt.Errorf("unterminated template literal")
	}

	var tokenizeSegment func() ([]token, error)
	var nextToken func() (*token, error)

	tokenizeSegment = func() ([]token, error) {
		seg := []token{}
		depth := 1
		for pos < n && depth > 0 {
			ch := peek(0)
			if ch == '{' {
				depth++
				advance()
				t := token{tokLeftBrace, "{", pos - 1}
				seg = append(seg, t)
				lastType = t.typ
				continue
			}
			if ch == '}' {
				depth--
				if depth == 0 {
					advance()
					break
				}
				advance()
				t := token{tokRightBrace, "}", pos - 1}
				seg = append(seg, t)
				lastType = t.typ
				continue
			}
			saved := pos
			t, err := nextToken()
			if err != nil {
				return nil, err
			}
			if t != nil {
				seg = append(seg, *t)
				lastType = t.typ
			} else if pos == saved {
				advance()
			}
		}
		return seg, nil
	}

	readTemplate := func(start int) error {
		content, ended, _, err := readTemplateContent()
		if err != nil {
			return err
		}
		if ended {
			tokens = append(tokens, token{tokTemplateLiteral, content, start})
			return nil
		}
		tokens = append(tokens, token{tokTemplateHead, content, start})
		seg, err := tokenizeSegment()
		if err != nil {
			return err
		}
		tokens = append(tokens, seg...)
		for {
			partContent, partEnded, _, err := readTemplateContent()
			if err != nil {
				return err
			}
			if partEnded {
				tokens = append(tokens, token{tokTemplateTail, partContent, pos})
				break
			}
			tokens = append(tokens, token{tokTemplateMiddle, partContent, pos})
			seg, err = tokenizeSegment()
			if err != nil {
				return err
			}
			tokens = append(tokens, seg...)
		}
		return nil
	}

	isDigit := func(ch byte) bool { return ch >= '0' && ch <= '9' }
	isAlpha := func(ch byte) bool {
		return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
	}
	isAlNum := func(ch byte) bool { return isAlpha(ch) || isDigit(ch) }
	isSpace := func(ch byte) bool { return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' }
	isRegexFlag := func(ch byte) bool { return ch == 'i' || ch == 'm' || ch == 's' || ch == 'g' || ch == 'u' }

	readRegex := func(start int) (token, error) {
		pattern := []byte{}
		inClass := false
		for pos < n {
			ch := advance()
			if ch == '\n' {
				return token{}, fmt.Errorf("unterminated regex literal at position %d", start)
			}
			if ch == '\\' {
				esc := advance()
				pattern = append(pattern, '\\', esc)
				continue
			}
			if ch == '[' {
				inClass = true
				pattern = append(pattern, ch)
				continue
			}
			if ch == ']' {
				inClass = false
				pattern = append(pattern, ch)
				continue
			}
			if ch == '/' && !inClass {
				flags := []byte{}
				for pos < n && isRegexFlag(peek(0)) {
					flags = append(flags, advance())
				}
				val := string(pattern) + "/" + string(flags)
				t := token{tokRegex, val, start}
				return t, nil
			}
			pattern = append(pattern, ch)
		}
		return token{}, fmt.Errorf("unterminated regex literal at position %d", start)
	}

	nextToken = func() (*token, error) {
		for pos < n && isSpace(peek(0)) {
			advance()
		}
		if pos >= n {
			return nil, nil
		}
		start := pos
		ch := peek(0)

		if isDigit(ch) {
			num := []byte{}
			for pos < n && isDigit(peek(0)) {
				num = append(num, advance())
			}
			if peek(0) == '.' && isDigit(peek(1)) {
				num = append(num, advance())
				for pos < n && isDigit(peek(0)) {
					num = append(num, advance())
				}
			}
			if peek(0) == 'e' || peek(0) == 'E' {
				num = append(num, advance())
				if peek(0) == '+' || peek(0) == '-' {
					num = append(num, advance())
				}
				for pos < n && isDigit(peek(0)) {
					num = append(num, advance())
				}
			}
			t := token{tokNumber, string(num), start}
			return &t, nil
		}

		if ch == '"' || ch == '\'' {
			advance()
			t, err := readString(ch, start)
			if err != nil {
				return nil, err
			}
			return &t, nil
		}

		if ch == '`' {
			advance()
			err := readTemplate(start)
			if err != nil {
				return nil, err
			}
			return nil, nil
		}

		if isAlpha(ch) {
			ident := []byte{}
			for pos < n && isAlNum(peek(0)) {
				ident = append(ident, advance())
			}
			s := string(ident)
			if s == "true" || s == "false" {
				t := token{tokBoolean, s, start}
				return &t, nil
			}
			if s == "null" {
				t := token{tokNull, s, start}
				return &t, nil
			}
			if s == "let" {
				t := token{tokLet, s, start}
				return &t, nil
			}
			t := token{tokIdentifier, s, start}
			return &t, nil
		}

		if pos+2 < n && src[pos] == '.' && src[pos+1] == '.' && src[pos+2] == '.' {
			pos += 3
			t := token{tokDotDotDot, "...", start}
			return &t, nil
		}

		two := ""
		if pos+1 < n {
			two = string(src[pos : pos+2])
		}
		switch two {
		case "**":
			pos += 2
			t := token{tokStarStar, "**", start}
			return &t, nil
		case "==":
			pos += 2
			t := token{tokEqualEqual, "==", start}
			return &t, nil
		case "!=":
			pos += 2
			t := token{tokBangEqual, "!=", start}
			return &t, nil
		case "<=":
			pos += 2
			t := token{tokLessEqual, "<=", start}
			return &t, nil
		case ">=":
			pos += 2
			t := token{tokGreaterEqual, ">=", start}
			return &t, nil
		case "&&":
			pos += 2
			t := token{tokAmpAmp, "&&", start}
			return &t, nil
		case "||":
			pos += 2
			t := token{tokPipePipe, "||", start}
			return &t, nil
		case "??":
			pos += 2
			t := token{tokQuestionQuestion, "??", start}
			return &t, nil
		case "?.":
			pos += 2
			t := token{tokQuestionDot, "?.", start}
			return &t, nil
		case "|>":
			pos += 2
			t := token{tokPipeGreater, "|>", start}
			return &t, nil
		case "=>":
			pos += 2
			t := token{tokArrow, "=>", start}
			return &t, nil
		}

		if ch == '/' && (lastType == -1 || regexAfterTokens[lastType]) {
			advance()
			t, err := readRegex(start)
			if err != nil {
				return nil, err
			}
			return &t, nil
		}
		advance()
		var typ tokenType
		switch ch {
		case '+':
			typ = tokPlus
		case '-':
			typ = tokMinus
		case '*':
			typ = tokStar
		case '/':
			typ = tokSlash
		case '%':
			typ = tokPercent
		case '!':
			typ = tokBang
		case '<':
			typ = tokLess
		case '>':
			typ = tokGreater
		case '?':
			typ = tokQuestion
		case ':':
			typ = tokColon
		case ',':
			typ = tokComma
		case '.':
			typ = tokDot
		case '(':
			typ = tokLeftParen
		case ')':
			typ = tokRightParen
		case '[':
			typ = tokLeftBracket
		case ']':
			typ = tokRightBracket
		case '{':
			typ = tokLeftBrace
		case '}':
			typ = tokRightBrace
		case ';':
			typ = tokSemicolon
		case '=':
			typ = tokEqual
		default:
			return nil, fmt.Errorf("unexpected character '%c' at position %d", ch, start)
		}
		t := token{typ, string(ch), start}
		return &t, nil
	}

	for pos < n {
		for pos < n && isSpace(peek(0)) {
			advance()
		}
		if pos >= n {
			break
		}
		t, err := nextToken()
		if err != nil {
			return nil, err
		}
		if t != nil {
			tokens = append(tokens, *t)
			lastType = t.typ
		}
	}
	tokens = append(tokens, token{tokEOF, "", pos})
	return tokens, nil
}
