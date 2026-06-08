package huml

import "github.com/flowctl/flowctl/internal/model"

func ParseHUML(source string) (*model.WorkflowDefinition, error) {
	lexer := NewLexer(source)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, err
	}

	parser := NewParser(tokens)
	ast, err := parser.Parse()
	if err != nil {
		return nil, err
	}

	return Convert(ast)
}
