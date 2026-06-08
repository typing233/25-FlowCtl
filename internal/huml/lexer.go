package huml

type TokenType int

const (
	TokenEOF TokenType = iota
	TokenNewline
	TokenIndent
	TokenDedent
	TokenIdentifier
	TokenString
	TokenNumber
	TokenBool
	TokenNull
	TokenBlockStart
	TokenBlockEnd
	TokenAssign
	TokenColon
	TokenComma
	TokenDot
	TokenListStart
	TokenListEnd
	TokenExpression
	TokenComment
	TokenPipe
)

var tokenNames = map[TokenType]string{
	TokenEOF:        "EOF",
	TokenNewline:    "NEWLINE",
	TokenIndent:     "INDENT",
	TokenDedent:     "DEDENT",
	TokenIdentifier: "IDENTIFIER",
	TokenString:     "STRING",
	TokenNumber:     "NUMBER",
	TokenBool:       "BOOL",
	TokenNull:       "NULL",
	TokenBlockStart: "BLOCK_START",
	TokenBlockEnd:   "BLOCK_END",
	TokenAssign:     "ASSIGN",
	TokenColon:      "COLON",
	TokenComma:      "COMMA",
	TokenDot:        "DOT",
	TokenListStart:  "LIST_START",
	TokenListEnd:    "LIST_END",
	TokenExpression: "EXPRESSION",
	TokenComment:    "COMMENT",
	TokenPipe:       "PIPE",
}

func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return "UNKNOWN"
}

type Token struct {
	Type    TokenType
	Value   string
	Line    int
	Column  int
}

type Lexer struct {
	input   []rune
	pos     int
	line    int
	col     int
	tokens  []Token
}

func NewLexer(input string) *Lexer {
	return &Lexer{
		input: []rune(input),
		pos:   0,
		line:  1,
		col:   1,
	}
}

func (l *Lexer) Tokenize() ([]Token, error) {
	for l.pos < len(l.input) {
		ch := l.input[l.pos]

		switch {
		case ch == '#':
			l.skipComment()
		case ch == '\n':
			l.emit(TokenNewline, "\n")
			l.pos++
			l.line++
			l.col = 1
		case ch == ' ' || ch == '\t':
			l.pos++
			l.col++
		case ch == '{':
			l.emit(TokenBlockStart, "{")
			l.pos++
			l.col++
		case ch == '}':
			l.emit(TokenBlockEnd, "}")
			l.pos++
			l.col++
		case ch == '=':
			l.emit(TokenAssign, "=")
			l.pos++
			l.col++
		case ch == ':':
			l.emit(TokenColon, ":")
			l.pos++
			l.col++
		case ch == ',':
			l.emit(TokenComma, ",")
			l.pos++
			l.col++
		case ch == '.':
			l.emit(TokenDot, ".")
			l.pos++
			l.col++
		case ch == '[':
			l.emit(TokenListStart, "[")
			l.pos++
			l.col++
		case ch == ']':
			l.emit(TokenListEnd, "]")
			l.pos++
			l.col++
		case ch == '|':
			l.emit(TokenPipe, "|")
			l.pos++
			l.col++
		case ch == '"':
			tok, err := l.readString()
			if err != nil {
				return nil, err
			}
			l.tokens = append(l.tokens, tok)
		case ch == '$' && l.peek() == '{':
			tok, err := l.readExpression()
			if err != nil {
				return nil, err
			}
			l.tokens = append(l.tokens, tok)
		case isDigit(ch) || (ch == '-' && l.pos+1 < len(l.input) && isDigit(l.input[l.pos+1])):
			l.readNumber()
		case isLetter(ch) || ch == '_':
			l.readIdentifier()
		default:
			l.pos++
			l.col++
		}
	}

	l.emit(TokenEOF, "")
	return l.tokens, nil
}

func (l *Lexer) emit(tokenType TokenType, value string) {
	l.tokens = append(l.tokens, Token{
		Type:   tokenType,
		Value:  value,
		Line:   l.line,
		Column: l.col,
	})
}

func (l *Lexer) peek() rune {
	if l.pos+1 < len(l.input) {
		return l.input[l.pos+1]
	}
	return 0
}

func (l *Lexer) skipComment() {
	for l.pos < len(l.input) && l.input[l.pos] != '\n' {
		l.pos++
		l.col++
	}
}

func (l *Lexer) readString() (Token, error) {
	startLine := l.line
	startCol := l.col
	l.pos++ // skip opening quote
	l.col++

	var result []rune
	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if ch == '\\' && l.pos+1 < len(l.input) {
			l.pos++
			l.col++
			next := l.input[l.pos]
			switch next {
			case 'n':
				result = append(result, '\n')
			case 't':
				result = append(result, '\t')
			case '"':
				result = append(result, '"')
			case '\\':
				result = append(result, '\\')
			default:
				result = append(result, '\\', next)
			}
			l.pos++
			l.col++
			continue
		}
		if ch == '"' {
			l.pos++
			l.col++
			return Token{Type: TokenString, Value: string(result), Line: startLine, Column: startCol}, nil
		}
		if ch == '\n' {
			l.line++
			l.col = 0
		}
		result = append(result, ch)
		l.pos++
		l.col++
	}

	return Token{}, &ParseError{
		Line:    startLine,
		Column:  startCol,
		Message: "unterminated string literal",
	}
}

func (l *Lexer) readExpression() (Token, error) {
	startLine := l.line
	startCol := l.col
	l.pos += 2 // skip ${
	l.col += 2

	depth := 1
	var result []rune
	for l.pos < len(l.input) && depth > 0 {
		ch := l.input[l.pos]
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				l.pos++
				l.col++
				return Token{Type: TokenExpression, Value: string(result), Line: startLine, Column: startCol}, nil
			}
		}
		result = append(result, ch)
		l.pos++
		l.col++
	}

	return Token{}, &ParseError{
		Line:    startLine,
		Column:  startCol,
		Message: "unterminated expression",
	}
}

func (l *Lexer) readNumber() {
	startCol := l.col
	start := l.pos
	hasDecimal := false

	if l.input[l.pos] == '-' {
		l.pos++
		l.col++
	}

	for l.pos < len(l.input) {
		ch := l.input[l.pos]
		if isDigit(ch) {
			l.pos++
			l.col++
		} else if ch == '.' && !hasDecimal {
			hasDecimal = true
			l.pos++
			l.col++
		} else {
			break
		}
	}

	l.tokens = append(l.tokens, Token{
		Type:   TokenNumber,
		Value:  string(l.input[start:l.pos]),
		Line:   l.line,
		Column: startCol,
	})
}

func (l *Lexer) readIdentifier() {
	startCol := l.col
	start := l.pos

	for l.pos < len(l.input) && (isLetter(l.input[l.pos]) || isDigit(l.input[l.pos]) || l.input[l.pos] == '_' || l.input[l.pos] == '-') {
		l.pos++
		l.col++
	}

	value := string(l.input[start:l.pos])

	switch value {
	case "true", "false":
		l.tokens = append(l.tokens, Token{Type: TokenBool, Value: value, Line: l.line, Column: startCol})
	case "null", "nil":
		l.tokens = append(l.tokens, Token{Type: TokenNull, Value: value, Line: l.line, Column: startCol})
	default:
		l.tokens = append(l.tokens, Token{Type: TokenIdentifier, Value: value, Line: l.line, Column: startCol})
	}
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

func isLetter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}
