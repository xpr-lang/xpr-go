package xpr

import "fmt"

const (
	bpPipe     = 10
	bpTernary  = 20
	bpNullish  = 30
	bpOr       = 40
	bpAnd      = 50
	bpEquality = 60
	bpCompare  = 70
	bpAdd      = 80
	bpMul      = 90
	bpExp      = 100
	bpUnary    = 110
	bpPostfix  = 120
)

func leftBP(t token) int {
	switch t.typ {
	case tokPipeGreater:
		return bpPipe
	case tokQuestion:
		return bpTernary
	case tokQuestionQuestion:
		return bpNullish
	case tokPipePipe:
		return bpOr
	case tokAmpAmp:
		return bpAnd
	case tokEqualEqual, tokBangEqual:
		return bpEquality
	case tokLess, tokGreater, tokLessEqual, tokGreaterEqual:
		return bpCompare
	case tokPlus, tokMinus:
		return bpAdd
	case tokStar, tokSlash, tokPercent:
		return bpMul
	case tokStarStar:
		return bpExp
	case tokDot, tokQuestionDot, tokLeftBracket, tokLeftParen:
		return bpPostfix
	}
	return 0
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return token{tokEOF, "", -1}
}

func (p *parser) advance() token {
	t := p.peek()
	if t.typ != tokEOF {
		p.pos++
	}
	return t
}

func (p *parser) expect(typ tokenType) (token, error) {
	t := p.peek()
	if t.typ != typ {
		return token{}, fmt.Errorf("expected token %d but got %d at position %d", typ, t.typ, t.position)
	}
	return p.advance(), nil
}

func (p *parser) parseArgList() ([]*node, error) {
	args := []*node{}
	for p.peek().typ != tokRightParen && p.peek().typ != tokEOF {
		arg, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.peek().typ == tokComma {
			p.advance()
		} else {
			break
		}
	}
	return args, nil
}

func (p *parser) nud(t token) (*node, error) {
	pos := t.position

	switch t.typ {
	case tokNumber:
		var f float64
		fmt.Sscanf(t.value, "%g", &f)
		return &node{typ: nodeNumberLiteral, numVal: f, position: pos}, nil

	case tokString:
		return &node{typ: nodeStringLiteral, strVal: t.value, position: pos}, nil

	case tokBoolean:
		return &node{typ: nodeBooleanLiteral, boolVal: t.value == "true", position: pos}, nil

	case tokNull:
		return &node{typ: nodeNullLiteral, position: pos}, nil

	case tokTemplateLiteral:
		n := &node{typ: nodeTemplateLiteral, strSlice: []string{t.value}, children: []*node{}, position: pos}
		return n, nil

	case tokTemplateHead:
		quasis := []string{t.value}
		exprs := []*node{}
		for {
			expr, err := p.expression(0)
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, expr)
			nxt := p.peek()
			if nxt.typ == tokTemplateTail {
				quasis = append(quasis, p.advance().value)
				break
			} else if nxt.typ == tokTemplateMiddle {
				quasis = append(quasis, p.advance().value)
			} else {
				return nil, fmt.Errorf("unexpected token in template literal at position %d", nxt.position)
			}
		}
		return &node{typ: nodeTemplateLiteral, strSlice: quasis, children: exprs, position: pos}, nil

	case tokIdentifier:
		if p.peek().typ == tokArrow {
			p.advance()
			body, err := p.expression(0)
			if err != nil {
				return nil, err
			}
			return &node{typ: nodeArrowFunction, strSlice: []string{t.value}, children: []*node{body}, position: pos}, nil
		}
		return &node{typ: nodeIdentifier, strVal: t.value, position: pos}, nil

	case tokLeftParen:
		if p.peek().typ == tokRightParen {
			p.advance()
			if _, err := p.expect(tokArrow); err != nil {
				return nil, err
			}
			body, err := p.expression(0)
			if err != nil {
				return nil, err
			}
			return &node{typ: nodeArrowFunction, strSlice: []string{}, children: []*node{body}, position: pos}, nil
		}
		first, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if p.peek().typ == tokComma {
			if first.typ != nodeIdentifier {
				return nil, fmt.Errorf("arrow function params must be identifiers at position %d", pos)
			}
			params := []string{first.strVal}
			for p.peek().typ == tokComma {
				p.advance()
				pt, err := p.expect(tokIdentifier)
				if err != nil {
					return nil, err
				}
				params = append(params, pt.value)
			}
			if _, err := p.expect(tokRightParen); err != nil {
				return nil, err
			}
			if _, err := p.expect(tokArrow); err != nil {
				return nil, err
			}
			body, err := p.expression(0)
			if err != nil {
				return nil, err
			}
			return &node{typ: nodeArrowFunction, strSlice: params, children: []*node{body}, position: pos}, nil
		}
		if _, err := p.expect(tokRightParen); err != nil {
			return nil, err
		}
		if first.typ == nodeIdentifier && p.peek().typ == tokArrow {
			p.advance()
			body, err := p.expression(0)
			if err != nil {
				return nil, err
			}
			return &node{typ: nodeArrowFunction, strSlice: []string{first.strVal}, children: []*node{body}, position: pos}, nil
		}
		return first, nil

	case tokLet:
		nameTok, err := p.expect(tokIdentifier)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokEqual); err != nil {
			return nil, err
		}
		value, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokSemicolon); err != nil {
			return nil, err
		}
		if p.peek().typ == tokEOF {
			return nil, fmt.Errorf("expected expression after ';'")
		}
		body, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeLetExpression, strVal: nameTok.value, children: []*node{value, body}, position: pos}, nil

	case tokLeftBracket:
		elements := []*node{}
		for p.peek().typ != tokRightBracket && p.peek().typ != tokEOF {
			if p.peek().typ == tokDotDotDot {
				spreadPos := p.peek().position
				p.advance()
				arg, err := p.expression(0)
				if err != nil {
					return nil, err
				}
				elements = append(elements, &node{typ: nodeSpreadElement, children: []*node{arg}, position: spreadPos})
			} else {
				el, err := p.expression(0)
				if err != nil {
					return nil, err
				}
				elements = append(elements, el)
			}
			if p.peek().typ == tokComma {
				p.advance()
			} else {
				break
			}
		}
		if _, err := p.expect(tokRightBracket); err != nil {
			return nil, err
		}
		return &node{typ: nodeArrayExpression, children: elements, position: pos}, nil

	case tokLeftBrace:
		keys := []string{}
		vals := []*node{}
		for p.peek().typ != tokRightBrace && p.peek().typ != tokEOF {
			if p.peek().typ == tokDotDotDot {
				p.advance()
				val, err := p.expression(0)
				if err != nil {
					return nil, err
				}
				keys = append(keys, "...")
				vals = append(vals, val)
			} else {
				kt := p.peek()
				var key string
				if kt.typ == tokIdentifier || kt.typ == tokString {
					key = p.advance().value
				} else {
					return nil, fmt.Errorf("expected object key at position %d", kt.position)
				}
				if _, err := p.expect(tokColon); err != nil {
					return nil, err
				}
				val, err := p.expression(0)
				if err != nil {
					return nil, err
				}
				keys = append(keys, key)
				vals = append(vals, val)
			}
			if p.peek().typ == tokComma {
				p.advance()
			} else {
				break
			}
		}
		if _, err := p.expect(tokRightBrace); err != nil {
			return nil, err
		}
		return &node{typ: nodeObjectExpression, strSlice: keys, propVals: vals, position: pos}, nil

	case tokBang:
		arg, err := p.expression(bpUnary)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeUnaryExpression, strVal: "!", children: []*node{arg}, position: pos}, nil

	case tokMinus:
		arg, err := p.expression(bpUnary)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeUnaryExpression, strVal: "-", children: []*node{arg}, position: pos}, nil
	}

	return nil, fmt.Errorf("unexpected token %d ('%s') at position %d", t.typ, t.value, pos)
}

func (p *parser) led(left *node, t token) (*node, error) {
	pos := t.position

	switch t.typ {
	case tokPlus, tokMinus:
		right, err := p.expression(bpAdd)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeBinaryExpression, strVal: t.value, children: []*node{left, right}, position: pos}, nil

	case tokStar, tokSlash, tokPercent:
		right, err := p.expression(bpMul)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeBinaryExpression, strVal: t.value, children: []*node{left, right}, position: pos}, nil

	case tokStarStar:
		right, err := p.expression(bpExp - 1)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeBinaryExpression, strVal: "**", children: []*node{left, right}, position: pos}, nil

	case tokEqualEqual, tokBangEqual:
		right, err := p.expression(bpEquality)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeBinaryExpression, strVal: t.value, children: []*node{left, right}, position: pos}, nil

	case tokLess, tokGreater, tokLessEqual, tokGreaterEqual:
		right, err := p.expression(bpCompare)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeBinaryExpression, strVal: t.value, children: []*node{left, right}, position: pos}, nil

	case tokAmpAmp:
		right, err := p.expression(bpAnd)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeLogicalExpression, strVal: "&&", children: []*node{left, right}, position: pos}, nil

	case tokPipePipe:
		right, err := p.expression(bpOr)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeLogicalExpression, strVal: "||", children: []*node{left, right}, position: pos}, nil

	case tokQuestionQuestion:
		right, err := p.expression(bpNullish)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeLogicalExpression, strVal: "??", children: []*node{left, right}, position: pos}, nil

	case tokDot:
		prop, err := p.expect(tokIdentifier)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeMemberExpression, strVal: prop.value, computed: false, optional: false, children: []*node{left}, position: pos}, nil

	case tokQuestionDot:
		if p.peek().typ == tokLeftParen {
			p.advance()
			args, err := p.parseArgList()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tokRightParen); err != nil {
				return nil, err
			}
			callArgs := append([]*node{left}, args...)
			return &node{typ: nodeCallExpression, optional: true, children: callArgs, position: pos}, nil
		}
		prop, err := p.expect(tokIdentifier)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeMemberExpression, strVal: prop.value, computed: false, optional: true, children: []*node{left}, position: pos}, nil

	case tokLeftBracket:
		index, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokRightBracket); err != nil {
			return nil, err
		}
		return &node{typ: nodeMemberExpression, computed: true, optional: false, children: []*node{left, index}, position: pos}, nil

	case tokLeftParen:
		args, err := p.parseArgList()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokRightParen); err != nil {
			return nil, err
		}
		callArgs := append([]*node{left}, args...)
		return &node{typ: nodeCallExpression, optional: false, children: callArgs, position: pos}, nil

	case tokPipeGreater:
		right, err := p.expression(bpPipe)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodePipeExpression, children: []*node{left, right}, position: pos}, nil

	case tokQuestion:
		consequent, err := p.expression(0)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokColon); err != nil {
			return nil, err
		}
		alternate, err := p.expression(bpTernary - 1)
		if err != nil {
			return nil, err
		}
		return &node{typ: nodeConditionalExpression, children: []*node{left, consequent, alternate}, position: pos}, nil
	}

	return nil, fmt.Errorf("unexpected infix token %d at position %d", t.typ, pos)
}

func (p *parser) expression(rbp int) (*node, error) {
	t := p.advance()
	if t.typ == tokEOF {
		return nil, fmt.Errorf("unexpected end of expression")
	}
	left, err := p.nud(t)
	if err != nil {
		return nil, err
	}
	for rbp < leftBP(p.peek()) {
		op := p.advance()
		left, err = p.led(left, op)
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func parseTokens(tokens []token) (*node, error) {
	p := &parser{tokens: tokens}
	if p.peek().typ == tokEOF {
		return nil, fmt.Errorf("empty expression")
	}
	expr, err := p.expression(0)
	if err != nil {
		return nil, err
	}
	if p.peek().typ != tokEOF {
		t := p.peek()
		return nil, fmt.Errorf("unexpected token at position %d", t.position)
	}
	return expr, nil
}
