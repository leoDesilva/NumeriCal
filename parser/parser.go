package parser

import (
	"errors"
	"numerical/lexer"
	"strconv"
)

/* ---------------------------- Parser Structure ---------------------------- */

type Parser struct {
	tokens   []lexer.Token
	token    lexer.Token
	position int
}

func NewParser(tokens []lexer.Token) *Parser {
	p := &Parser{tokens: tokens, token: tokens[0], position: 0}
	return p
}

func (p *Parser) advance() lexer.Token {
	p.position++
	p.token = p.tokens[p.position]
	return p.tokens[p.position]
}

func (p *Parser) peekToken() lexer.Token {
	if p.position+1 >= len(p.tokens) {
		return lexer.Token{Type: lexer.EOF, Literal: ""}
	}
	return p.tokens[p.position+1]
}

func (p *Parser) Parse() (ProgramNode, error) {
	ast := ProgramNode{Nodes: make([]Node, 0)}
	for p.token.Type != lexer.EOF {
		if p.token.Type == lexer.SEMICOLON {
			p.advance()
		}
		node, err := p.parseExpression()
		if err != nil {
			return ProgramNode{}, err
		}
		ast.Nodes = append(ast.Nodes, node)
	}
	return ast, nil
}

func (p *Parser) parseExpression() (Node, error) {
	if p.token.Type == lexer.IDENTIFIER && p.peekToken().Type == lexer.EQ {
		expr, err := p.parseAssignment()
		if err != nil {
			return &ErrorNode{}, err
		}
		return expr, nil
	}

	if p.token.Type == lexer.DEFINE {
		expr, err := p.parseFunctionDefenition()
		if err != nil {
			return &ErrorNode{}, err
		}
		return expr, nil
	}

	expr, err := p.parseExpr(0)
	if err != nil {
		return &ErrorNode{}, err
	}
	return expr, nil
}

func (p *Parser) parseFunctionDefenition() (Node, error) {
	var err error
	p.advance()
	identifer := p.token.Literal
	p.advance()
	params := make([]Node, 0)
	if p.token.Type == lexer.LPAREN {
		params, err = p.parseParameters(lexer.RPAREN)
		if err != nil {
			return &ErrorNode{}, err
		}
		p.advance()
	}

	if p.token.Type != lexer.ARROW {
		return &ErrorNode{}, errors.New("SyntaxError: expected => while parsing functionDefenition")
	}

	p.advance()
	consequence := ProgramNode{Nodes: make([]Node, 0)}
	for p.token.Type != lexer.EOF {
		if p.token.Type == lexer.SEMICOLON {
			p.advance()
		}
		expr, err := p.parseExpression()
		if err != nil {
			return &ErrorNode{}, nil
		}
		consequence.Nodes = append(consequence.Nodes, expr)
	}

	return &FunctionDefenitionNode{identifer, params, consequence}, nil

}

func (p *Parser) parseAssignment() (Node, error) {
	identifier := p.token.Literal
	p.advance()
	p.advance()
	expr, err := p.parseExpr(0)
	if err != nil {
		return &ErrorNode{}, err
	}
	return &AssignNode{identifier, expr}, nil
}

func (p *Parser) parseExpr(rbp int) (Node, error) {
	left, err := p.parsePrefix()
	if err != nil {
		return &ErrorNode{}, err
	}
	peekRbp := preference(p.token.Type)
	for p.peekToken().Type != lexer.EOF && p.peekToken().Type != lexer.SEMICOLON && peekRbp >= rbp {
		left, err = p.parseInfix(left, p.token.Type)
		if err != nil {
			return &ErrorNode{}, err
		}
		peekRbp = preference(p.token.Type)
	}
	return left, nil
}

func (p *Parser) parsePrefix() (Node, error) {
	switch p.token.Type {
	case lexer.TILDE:
		p.advance()
		expression, err := p.parsePrefix()
		if err != nil {
			return &ErrorNode{}, err
		}
		return &UnaryOpNode{lexer.TILDE, expression}, nil

	case lexer.NOT:
		p.advance()
		expression, err := p.parsePrefix()
		if err != nil {
			return &ErrorNode{}, err
		}
		return &UnaryOpNode{lexer.NOT, expression}, nil

	case lexer.SUB:
		p.advance()
		expression, err := p.parsePrefix()
		if err != nil {
			return &ErrorNode{}, err
		}
		return &UnaryOpNode{lexer.SUB, expression}, nil

	case lexer.LPAREN:
		p.advance()
		expression, err := p.parseExpr(preference(lexer.LPAREN))
		if err != nil {
			return &ErrorNode{}, err
		}
		p.advance()
		if p.token.Type == lexer.IDENTIFIER {
			unit := p.token.Literal
			p.advance()
			return &UnitNode{expression, unit}, nil
		}
		return expression, nil

	case lexer.LSQUARE:
		nodes, err := p.parseParameters(lexer.RSQUARE)
		if err != nil {
			return &ErrorNode{}, err
		}
		p.advance()
		return &ArrayNode{nodes}, nil

	case lexer.IDENTIFIER:
		identifier := p.token.Literal
		p.advance()
		if p.token.Type == lexer.LPAREN {
			params, err := p.parseParameters(lexer.RPAREN)
			if err != nil {
				return &ErrorNode{}, nil
			}
			p.advance()
			return &FunctionCallNode{identifier, ProgramNode{params}}, nil
		}

		if p.token.Type == lexer.IDENTIFIER {
			unit := p.token.Literal
			p.advance()
			return &UnitNode{&IdentifierNode{Identifier: identifier}, unit}, nil
		}
		return &IdentifierNode{identifier}, nil

	case lexer.INT:
		value, err := strconv.Atoi(p.token.Literal)
		if err != nil {
			return &ErrorNode{}, err
		}
		p.advance()
		node := &IntNode{Value: value}
		if p.token.Type == lexer.IDENTIFIER {
			unit := p.token.Literal
			p.advance()
			return &UnitNode{node, unit}, nil
		}
		return node, nil

	case lexer.FLOAT:
		value, err := strconv.ParseFloat(p.token.Literal, 64)
		if err != nil {
			return &ErrorNode{}, err
		}
		p.advance()
		node := &FloatNode{Value: value}
		if p.token.Type == lexer.IDENTIFIER {
			unit := p.token.Literal
			p.advance()
			return &UnitNode{node, unit}, nil
		}
		return node, nil

	case lexer.STRING:
		value := p.token.Literal
		p.advance()
		return &StringNode{value}, nil
	}
	return &ErrorNode{}, errors.New("SyntaxError: parsePrefix() unsupported prefix:" + p.token.Literal)
}

func (p *Parser) parseInfix(left Node, operation string) (Node, error) {
	if !contains([]string{
		"EE", "NE", "LT", "GT", "LTE", "GTE", "ADD", "SUB", "MUL", "DIV", "MOD", "POW", "IN", "ARROW",
	}, p.token.Type) {
		return &ErrorNode{}, errors.New("SyntaxError: parseInfix() unsupported opperator:" + p.token.Literal)
	}

	p.advance()
	right, err := p.parseExpr(preference(operation) + 1)

	if err != nil {
		return &ErrorNode{}, err
	}
	if operation == lexer.ARROW {
		operation = lexer.IN
	}
	return &BinOpNode{Left: left, Operation: operation, Right: right}, nil
}

func (p *Parser) parseParameters(terminate string) ([]Node, error) {
	parameters := make([]Node, 0)
	p.advance()
	if p.token.Type == lexer.RPAREN {
		return parameters, nil
	}
	for p.token.Type != terminate {
		if p.token.Type == lexer.EOF || p.token.Type == lexer.SEMICOLON {
			return make([]Node, 0), errors.New("SyntaxError: Unclosed parenthesis parseParameters()")
		}
		if p.token.Type != lexer.COMMA {
			param, err := p.parseExpr(0)
			if err != nil {
				return make([]Node, 0), err
			}
			parameters = append(parameters, param)
		} else {
			p.advance()
		}
	}
	return parameters, nil
}

func preference(tokenType string) int {
	var preferences = map[string]int{
		lexer.IN:     5,
		lexer.ARROW:  5,
		lexer.EE:     10,
		lexer.NE:     10,
		lexer.GT:     10,
		lexer.GTE:    10,
		lexer.LT:     10,
		lexer.LTE:    10,
		lexer.MOD:    15,
		lexer.ADD:    20,
		lexer.SUB:    20,
		lexer.MUL:    30,
		lexer.DIV:    30,
		lexer.POW:    40,
		lexer.LPAREN: 0,
	}

	if rbp, ok := preferences[tokenType]; ok {
		return rbp
	}
	return -1
}

func contains(array []string, element string) bool {
	for _, e := range array {
		if e == element {
			return true
		}
	}
	return false
}
