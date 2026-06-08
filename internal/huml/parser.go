package huml

import (
	"fmt"
	"strconv"
)

type Parser struct {
	tokens  []Token
	pos     int
	errors  []error
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens, pos: 0}
}

func (p *Parser) Parse() (*WorkflowNode, error) {
	p.skipNewlines()

	if !p.matchIdentifier("workflow") {
		return nil, p.errorf("expected 'workflow' keyword")
	}

	name := ""
	if p.current().Type == TokenString {
		name = p.current().Value
		p.advance()
	}

	if err := p.expect(TokenBlockStart); err != nil {
		return nil, err
	}

	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}

	if err := p.expect(TokenBlockEnd); err != nil {
		return nil, err
	}

	return &WorkflowNode{
		Name: name,
		Line: 1,
		Column: 1,
		Body: body,
	}, nil
}

func (p *Parser) parseBody() ([]Node, error) {
	var nodes []Node
	p.skipNewlines()

	for p.pos < len(p.tokens) && p.current().Type != TokenBlockEnd && p.current().Type != TokenEOF {
		node, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		if node != nil {
			nodes = append(nodes, node)
		}
		p.skipNewlines()
	}

	return nodes, nil
}

func (p *Parser) parseStatement() (Node, error) {
	tok := p.current()

	if tok.Type == TokenIdentifier {
		next := p.peekToken()

		// Block: `step "name" {`, `input "name" {`, `approval "name" {`, `retry {`, `compensation {`
		if isBlockKeyword(tok.Value) {
			return p.parseBlock()
		}

		// Assignment: `key = value`
		if next.Type == TokenAssign {
			return p.parseAssignment()
		}

		// Standalone identifier (shouldn't happen in well-formed HUML)
		p.advance()
		return &IdentifierNode{Value: tok.Value, Line: tok.Line, Column: tok.Column}, nil
	}

	if tok.Type == TokenNewline {
		p.advance()
		return nil, nil
	}

	p.advance()
	return nil, nil
}

func (p *Parser) parseBlock() (Node, error) {
	blockType := p.current().Value
	line := p.current().Line
	col := p.current().Column
	p.advance()

	name := ""
	if p.current().Type == TokenString {
		name = p.current().Value
		p.advance()
	}

	if err := p.expect(TokenBlockStart); err != nil {
		return nil, err
	}

	body, err := p.parseBody()
	if err != nil {
		return nil, err
	}

	if err := p.expect(TokenBlockEnd); err != nil {
		return nil, err
	}

	return &BlockNode{
		Type:   blockType,
		Name:   name,
		Line:   line,
		Column: col,
		Body:   body,
	}, nil
}

func (p *Parser) parseAssignment() (Node, error) {
	key := p.current().Value
	line := p.current().Line
	col := p.current().Column
	p.advance() // skip identifier
	p.advance() // skip '='

	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}

	return &AssignNode{
		Key:    key,
		Value:  value,
		Line:   line,
		Column: col,
	}, nil
}

func (p *Parser) parseValue() (Node, error) {
	tok := p.current()

	switch tok.Type {
	case TokenString:
		p.advance()
		return &StringNode{Value: tok.Value, Line: tok.Line, Column: tok.Column}, nil

	case TokenNumber:
		p.advance()
		return &NumberNode{Value: tok.Value, Line: tok.Line, Column: tok.Column}, nil

	case TokenBool:
		p.advance()
		return &BoolNode{Value: tok.Value == "true", Line: tok.Line, Column: tok.Column}, nil

	case TokenNull:
		p.advance()
		return &NullNode{Line: tok.Line, Column: tok.Column}, nil

	case TokenExpression:
		p.advance()
		expr, err := ParseExpression(tok.Value)
		if err != nil {
			return nil, &ParseError{Line: tok.Line, Column: tok.Column, Message: fmt.Sprintf("invalid expression: %v", err)}
		}
		return &ExpressionNode{Raw: tok.Value, Expr: expr, Line: tok.Line, Column: tok.Column}, nil

	case TokenListStart:
		return p.parseList()

	case TokenIdentifier:
		p.advance()
		return &IdentifierNode{Value: tok.Value, Line: tok.Line, Column: tok.Column}, nil

	default:
		return nil, p.errorf("unexpected token %s (%q)", tok.Type, tok.Value)
	}
}

func (p *Parser) parseList() (Node, error) {
	line := p.current().Line
	col := p.current().Column
	p.advance() // skip '['

	var items []Node
	p.skipNewlines()

	for p.current().Type != TokenListEnd && p.current().Type != TokenEOF {
		item, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, item)

		p.skipNewlines()
		if p.current().Type == TokenComma {
			p.advance()
			p.skipNewlines()
		}
	}

	if err := p.expect(TokenListEnd); err != nil {
		return nil, err
	}

	return &ListNode{Items: items, Line: line, Column: col}, nil
}

func (p *Parser) current() Token {
	if p.pos >= len(p.tokens) {
		return Token{Type: TokenEOF, Line: -1, Column: -1}
	}
	return p.tokens[p.pos]
}

func (p *Parser) peekToken() Token {
	if p.pos+1 >= len(p.tokens) {
		return Token{Type: TokenEOF}
	}
	return p.tokens[p.pos+1]
}

func (p *Parser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

func (p *Parser) expect(expected TokenType) error {
	if p.current().Type != expected {
		return p.errorf("expected %s, got %s (%q)", expected, p.current().Type, p.current().Value)
	}
	p.advance()
	return nil
}

func (p *Parser) matchIdentifier(name string) bool {
	if p.current().Type == TokenIdentifier && p.current().Value == name {
		p.advance()
		return true
	}
	return false
}

func (p *Parser) skipNewlines() {
	for p.pos < len(p.tokens) && p.current().Type == TokenNewline {
		p.advance()
	}
}

func (p *Parser) errorf(format string, args ...any) error {
	tok := p.current()
	return &ParseError{
		Line:    tok.Line,
		Column:  tok.Column,
		Message: fmt.Sprintf(format, args...),
	}
}

func isBlockKeyword(s string) bool {
	switch s {
	case "step", "input", "approval", "retry", "compensation", "workflow":
		return true
	}
	return false
}

// Expression parser (for ${...} interpolation)

type exprParser struct {
	input []rune
	pos   int
}

func ParseExpression(input string) (Expr, error) {
	ep := &exprParser{input: []rune(input), pos: 0}
	expr, err := ep.parseExpr()
	if err != nil {
		return nil, err
	}
	return expr, nil
}

func (ep *exprParser) parseExpr() (Expr, error) {
	return ep.parseTernary()
}

func (ep *exprParser) parseTernary() (Expr, error) {
	expr, err := ep.parseOr()
	if err != nil {
		return nil, err
	}

	ep.skipWhitespace()
	if ep.pos < len(ep.input) && ep.input[ep.pos] == '?' {
		ep.pos++
		ep.skipWhitespace()
		thenExpr, err := ep.parseExpr()
		if err != nil {
			return nil, err
		}
		ep.skipWhitespace()
		if ep.pos >= len(ep.input) || ep.input[ep.pos] != ':' {
			return nil, fmt.Errorf("expected ':' in ternary expression")
		}
		ep.pos++
		ep.skipWhitespace()
		elseExpr, err := ep.parseExpr()
		if err != nil {
			return nil, err
		}
		return &ConditionalExpr{Cond: expr, Then: thenExpr, Else: elseExpr}, nil
	}

	return expr, nil
}

func (ep *exprParser) parseOr() (Expr, error) {
	left, err := ep.parseAnd()
	if err != nil {
		return nil, err
	}

	for {
		ep.skipWhitespace()
		if ep.matchStr("||") {
			right, err := ep.parseAnd()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Op: "||", Left: left, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (ep *exprParser) parseAnd() (Expr, error) {
	left, err := ep.parseComparison()
	if err != nil {
		return nil, err
	}

	for {
		ep.skipWhitespace()
		if ep.matchStr("&&") {
			right, err := ep.parseComparison()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Op: "&&", Left: left, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (ep *exprParser) parseComparison() (Expr, error) {
	left, err := ep.parseAddition()
	if err != nil {
		return nil, err
	}

	ep.skipWhitespace()
	for _, op := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		if ep.matchStr(op) {
			right, err := ep.parseAddition()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Op: op, Left: left, Right: right}
			ep.skipWhitespace()
		}
	}
	return left, nil
}

func (ep *exprParser) parseAddition() (Expr, error) {
	left, err := ep.parseMultiplication()
	if err != nil {
		return nil, err
	}

	for {
		ep.skipWhitespace()
		if ep.pos < len(ep.input) && (ep.input[ep.pos] == '+' || ep.input[ep.pos] == '-') {
			op := string(ep.input[ep.pos])
			ep.pos++
			right, err := ep.parseMultiplication()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Op: op, Left: left, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (ep *exprParser) parseMultiplication() (Expr, error) {
	left, err := ep.parseUnary()
	if err != nil {
		return nil, err
	}

	for {
		ep.skipWhitespace()
		if ep.pos < len(ep.input) && (ep.input[ep.pos] == '*' || ep.input[ep.pos] == '/' || ep.input[ep.pos] == '%') {
			op := string(ep.input[ep.pos])
			ep.pos++
			right, err := ep.parseUnary()
			if err != nil {
				return nil, err
			}
			left = &BinaryExpr{Op: op, Left: left, Right: right}
		} else {
			break
		}
	}
	return left, nil
}

func (ep *exprParser) parseUnary() (Expr, error) {
	ep.skipWhitespace()
	if ep.pos < len(ep.input) && ep.input[ep.pos] == '!' {
		ep.pos++
		operand, err := ep.parseUnary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: "!", Operand: operand}, nil
	}
	return ep.parsePrimary()
}

func (ep *exprParser) parsePrimary() (Expr, error) {
	ep.skipWhitespace()

	if ep.pos >= len(ep.input) {
		return nil, fmt.Errorf("unexpected end of expression")
	}

	ch := ep.input[ep.pos]

	// Parenthesized expression
	if ch == '(' {
		ep.pos++
		expr, err := ep.parseExpr()
		if err != nil {
			return nil, err
		}
		ep.skipWhitespace()
		if ep.pos >= len(ep.input) || ep.input[ep.pos] != ')' {
			return nil, fmt.Errorf("expected ')'")
		}
		ep.pos++
		return expr, nil
	}

	// String literal
	if ch == '\'' || ch == '"' {
		return ep.parseStringLiteral()
	}

	// Number
	if isDigit(ch) || (ch == '-' && ep.pos+1 < len(ep.input) && isDigit(ep.input[ep.pos+1])) {
		return ep.parseNumberLiteral()
	}

	// Identifier / reference / function call
	if isLetter(ch) || ch == '_' {
		return ep.parseRef()
	}

	return nil, fmt.Errorf("unexpected character '%c'", ch)
}

func (ep *exprParser) parseRef() (Expr, error) {
	var path []string
	ident := ep.readIdent()

	// Check for boolean literals
	if ident == "true" {
		return &LiteralExpr{Value: true}, nil
	}
	if ident == "false" {
		return &LiteralExpr{Value: false}, nil
	}
	if ident == "null" || ident == "nil" {
		return &LiteralExpr{Value: nil}, nil
	}

	// Check for function call
	if ep.pos < len(ep.input) && ep.input[ep.pos] == '(' {
		return ep.parseFuncCall(ident)
	}

	path = append(path, ident)

	// Dot-access chain
	for ep.pos < len(ep.input) && ep.input[ep.pos] == '.' {
		ep.pos++
		next := ep.readIdent()
		if next == "" {
			return nil, fmt.Errorf("expected identifier after '.'")
		}
		path = append(path, next)
	}

	return &RefExpr{Path: path}, nil
}

func (ep *exprParser) parseFuncCall(name string) (Expr, error) {
	ep.pos++ // skip '('
	var args []Expr

	ep.skipWhitespace()
	if ep.pos < len(ep.input) && ep.input[ep.pos] == ')' {
		ep.pos++
		return &FuncCallExpr{Name: name, Args: args}, nil
	}

	for {
		ep.skipWhitespace()
		arg, err := ep.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		ep.skipWhitespace()
		if ep.pos < len(ep.input) && ep.input[ep.pos] == ',' {
			ep.pos++
		} else {
			break
		}
	}

	ep.skipWhitespace()
	if ep.pos >= len(ep.input) || ep.input[ep.pos] != ')' {
		return nil, fmt.Errorf("expected ')' after function arguments")
	}
	ep.pos++
	return &FuncCallExpr{Name: name, Args: args}, nil
}

func (ep *exprParser) parseStringLiteral() (Expr, error) {
	quote := ep.input[ep.pos]
	ep.pos++
	var result []rune
	for ep.pos < len(ep.input) {
		ch := ep.input[ep.pos]
		if ch == '\\' && ep.pos+1 < len(ep.input) {
			ep.pos++
			result = append(result, ep.input[ep.pos])
			ep.pos++
			continue
		}
		if ch == quote {
			ep.pos++
			return &LiteralExpr{Value: string(result)}, nil
		}
		result = append(result, ch)
		ep.pos++
	}
	return nil, fmt.Errorf("unterminated string")
}

func (ep *exprParser) parseNumberLiteral() (Expr, error) {
	start := ep.pos
	if ep.input[ep.pos] == '-' {
		ep.pos++
	}
	for ep.pos < len(ep.input) && (isDigit(ep.input[ep.pos]) || ep.input[ep.pos] == '.') {
		ep.pos++
	}
	numStr := string(ep.input[start:ep.pos])
	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid number: %s", numStr)
	}
	return &LiteralExpr{Value: val}, nil
}

func (ep *exprParser) readIdent() string {
	start := ep.pos
	for ep.pos < len(ep.input) && (isLetter(ep.input[ep.pos]) || isDigit(ep.input[ep.pos]) || ep.input[ep.pos] == '_') {
		ep.pos++
	}
	return string(ep.input[start:ep.pos])
}

func (ep *exprParser) skipWhitespace() {
	for ep.pos < len(ep.input) && (ep.input[ep.pos] == ' ' || ep.input[ep.pos] == '\t') {
		ep.pos++
	}
}

func (ep *exprParser) matchStr(s string) bool {
	if ep.pos+len(s) > len(ep.input) {
		return false
	}
	for i, c := range s {
		if ep.input[ep.pos+i] != c {
			return false
		}
	}
	ep.pos += len(s)
	return true
}
