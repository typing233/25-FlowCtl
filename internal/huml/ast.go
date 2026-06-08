package huml

import "fmt"

type ParseError struct {
	Line    int
	Column  int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d, col %d: %s", e.Line, e.Column, e.Message)
}

// AST Node types

type Node interface {
	nodeType() string
	Pos() (int, int)
}

type WorkflowNode struct {
	Name    string
	Line    int
	Column  int
	Body    []Node
}

func (n *WorkflowNode) nodeType() string { return "workflow" }
func (n *WorkflowNode) Pos() (int, int)  { return n.Line, n.Column }

type AssignNode struct {
	Key    string
	Value  Node
	Line   int
	Column int
}

func (n *AssignNode) nodeType() string { return "assign" }
func (n *AssignNode) Pos() (int, int)  { return n.Line, n.Column }

type BlockNode struct {
	Type   string // "step", "input", "approval", "retry", "compensation"
	Name   string
	Line   int
	Column int
	Body   []Node
}

func (n *BlockNode) nodeType() string { return "block" }
func (n *BlockNode) Pos() (int, int)  { return n.Line, n.Column }

type StringNode struct {
	Value  string
	Line   int
	Column int
}

func (n *StringNode) nodeType() string { return "string" }
func (n *StringNode) Pos() (int, int)  { return n.Line, n.Column }

type NumberNode struct {
	Value  string
	Line   int
	Column int
}

func (n *NumberNode) nodeType() string { return "number" }
func (n *NumberNode) Pos() (int, int)  { return n.Line, n.Column }

type BoolNode struct {
	Value  bool
	Line   int
	Column int
}

func (n *BoolNode) nodeType() string { return "bool" }
func (n *BoolNode) Pos() (int, int)  { return n.Line, n.Column }

type NullNode struct {
	Line   int
	Column int
}

func (n *NullNode) nodeType() string { return "null" }
func (n *NullNode) Pos() (int, int)  { return n.Line, n.Column }

type ListNode struct {
	Items  []Node
	Line   int
	Column int
}

func (n *ListNode) nodeType() string { return "list" }
func (n *ListNode) Pos() (int, int)  { return n.Line, n.Column }

type ExpressionNode struct {
	Raw    string
	Expr   Expr
	Line   int
	Column int
}

func (n *ExpressionNode) nodeType() string { return "expression" }
func (n *ExpressionNode) Pos() (int, int)  { return n.Line, n.Column }

type IdentifierNode struct {
	Value  string
	Line   int
	Column int
}

func (n *IdentifierNode) nodeType() string { return "identifier" }
func (n *IdentifierNode) Pos() (int, int)  { return n.Line, n.Column }

// Expression AST

type Expr interface {
	exprType() string
}

type RefExpr struct {
	Path []string
}

func (e *RefExpr) exprType() string { return "ref" }

type LiteralExpr struct {
	Value any
}

func (e *LiteralExpr) exprType() string { return "literal" }

type BinaryExpr struct {
	Op    string
	Left  Expr
	Right Expr
}

func (e *BinaryExpr) exprType() string { return "binary" }

type UnaryExpr struct {
	Op      string
	Operand Expr
}

func (e *UnaryExpr) exprType() string { return "unary" }

type FuncCallExpr struct {
	Name string
	Args []Expr
}

func (e *FuncCallExpr) exprType() string { return "func_call" }

type ConditionalExpr struct {
	Cond Expr
	Then Expr
	Else Expr
}

func (e *ConditionalExpr) exprType() string { return "conditional" }

type IndexExpr struct {
	Object Expr
	Index  Expr
}

func (e *IndexExpr) exprType() string { return "index" }
