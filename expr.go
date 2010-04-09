package gotcl

import (
	"os"
	"io"
	"unicode"
)

type eterm interface {
	String() string
	Eval(*Interp) TclStatus
}


type binOp string
type binOpNode struct {
	op   binOp
	a, b eterm
}

type unOpNode struct {
	op string
	v  eterm
}

func (u *unOpNode) String() string {
	return "(" + u.op + " " + u.v.String() + ")"
}

func (u *unOpNode) Eval(i *Interp) TclStatus {
	rc := u.v.Eval(i)
	if rc != kTclOK {
		return rc
	}
	return i.Return(FromBool(!i.retval.AsBool()))
}


type parenNode struct {
	term eterm
}


func (p *parenNode) Eval(i *Interp) TclStatus {
	return p.term.Eval(i)
}

func (p *parenNode) String() string {
	return p.term.String()
}

func callCmd(i *Interp, name string, args ...*TclObj) TclStatus {
	c := i.cmds[name]
	if c == nil {
		return i.FailStr("Not a command: " + name)
	}
	return c(i, args)
}

func (bb *binOpNode) Eval(i *Interp) TclStatus {
	bb.a.Eval(i)
	a := i.retval
	bb.b.Eval(i)
	b := i.retval
	if i.err != nil {
		return i.Fail(i.err)
	}
	return callCmd(i, string(bb.op), a, b)
}

func (bb *binOpNode) String() string {
	return "(" + string(bb.op) + " " + bb.a.String() + " " + bb.b.String() + ")"
}

func gbalance(b eterm) eterm {
	bb, ok := b.(*binOpNode)
	if ok {
		return balance(bb)
	}
	return b
}

var oplevel = map[binOp]int{
	"*": 3, "/": 3,
	"+": 2, "-": 2,
	"==": 1, "!=": 1,
	"&&": 0, "||": 0}

func opgt(a, b binOp) bool {
	al, aok := oplevel[a]
	bl, bok := oplevel[b]
	if !aok || !bok {
		return false
	}
	return al >= bl
}

func balance(b *binOpNode) *binOpNode {
	bb, ok := b.b.(*binOpNode)
	if !ok {
		return b
	}
	if opgt(b.op, bb.op) {
		return &binOpNode{bb.op, &binOpNode{b.op, gbalance(b.a), gbalance(bb.a)}, gbalance(bb.b)}
	}
	return b
}

func ParseExpr(in io.Reader) (item eterm, err os.Error) {
	p := newParser(in)
	defer setError(&err)
	item = p.parseExpr()
	return
}

func (p *parser) parseExpr() eterm {
	res := p.parseExprTerm()
	p.eatWhile(isspace)
	if p.ch != -1 {
		if p.ch == ')' {
			return res
		}
		return p.parseBinOpNode(res)
	}
	return res
}

func istermchar(c int) bool {
	return unicode.IsDigit(c) || unicode.IsLetter(c) || c == '.' || c == '-'
}

func (p *parser) parseExprTerm() eterm {
	p.eatWhile(isspace)
	switch p.ch {
	case '(':
		p.advance()
		e := p.parseExpr()
		p.consumeRune(')')
		return &parenNode{e}
	case '$':
		return p.parseVarRef()
	case '!':
		return p.parseUnOpNode()
	case '[':
		return p.parseSubcommand()
	}
	txt := p.consumeWhile1(istermchar, "term")
	return &tliteral{strval: txt}
}

func (p *parser) parseBinOp() binOp {
	switch p.ch {
	case '*':
		p.advance()
		return "*"
	case '+':
		p.advance()
		return "+"
	case '-':
		p.advance()
		return "-"
	case '|':
		p.advance()
		p.consumeRune('|')
		return "||"
	case '&':
		p.advance()
		p.consumeRune('&')
		return "&&"
	case '!':
		p.advance()
		p.consumeRune('=')
		return "!="
	case '=':
		p.advance()
		p.consumeRune('=')
		return "=="
	case '>':
		p.advance()
		if p.ch == '=' {
			p.advance()
			return ">="
		}
		return ">"
	case '<':
		p.advance()
		if p.ch == '=' {
			p.advance()
			return "<="
		}
		return "<"
	case -1:
		p.fail("EOF")
	}
	p.fail("expected binary operator, got " + string(p.ch))
	return ""
}

func (p *parser) parseUnOpNode() *unOpNode {
	p.eatWhile(isspace)
	p.consumeRune('!')
	return &unOpNode{"!", p.parseExprTerm()}
}

func (p *parser) parseBinOpNode(a eterm) *binOpNode {
	op := p.parseBinOp()
	p.eatWhile(isspace)
	return balance(&binOpNode{op, a, p.parseExpr()})
}

func tclExpr(i *Interp, args []*TclObj) TclStatus {
	if len(args) == 0 {
		return i.FailStr("wrong # args")
	}
	var expr eterm
	var err os.Error
	if len(args) == 1 {
		expr, err = args[0].asExpr()
	} else {
		expr, err = concat(args).asExpr()
	}
	if err != nil {
		return i.Fail(err)
	}
	return expr.Eval(i)
}
