//Copyright 2012 Vastech SA (PTY) LTD
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

package golang

import (
	"bytes"
	"path"
	"text/template"

	"github.com/goccmack/gocc/internal/ast"
	"github.com/goccmack/gocc/internal/config"
	"github.com/goccmack/gocc/internal/io"
	"github.com/goccmack/gocc/internal/parser/lr1/items"
	"github.com/goccmack/gocc/internal/parser/symbols"
)

func GenParser(pkg, outDir string, prods ast.SyntaxProdList, itemSets *items.ItemSets, symbols *symbols.Symbols, cfg config.Config) {
	tmpl, err := template.New("parser").Parse(parserSrc[1:])
	if err != nil {
		panic(err)
	}
	wr := new(bytes.Buffer)
	if err := tmpl.Execute(wr, getParserData(pkg, prods, itemSets, symbols, cfg)); err != nil {
		panic(err)
	}
	io.WriteFile(path.Join(outDir, "parser", "parser.go"), wr.Bytes())
}

type parserData struct {
	Debug          bool
	ErrorImport    string
	TokenImport    string
	NumProductions int
	NumStates      int
	NumSymbols     int
}

func getParserData(pkg string, prods ast.SyntaxProdList, itemSets *items.ItemSets, symbols *symbols.Symbols, cfg config.Config) *parserData {
	return &parserData{
		Debug:          cfg.DebugParser(),
		ErrorImport:    path.Join(pkg, "errors"),
		TokenImport:    path.Join(pkg, "token"),
		NumProductions: len(prods),
		NumStates:      itemSets.Size(),
		NumSymbols:     symbols.NumSymbols(),
	}
}

const parserSrc = `
// Code generated by gocc; DO NOT EDIT.

package parser

import (
	"bytes"
	"fmt"

	parseError "{{.ErrorImport}}"
	"{{.TokenImport}}"
)

const (
	numProductions = {{.NumProductions}}
	numStates      = {{.NumStates}}
	numSymbols     = {{.NumSymbols}}
)

// Stack

type stack struct {
	state  []int
	attrib []Attrib
}

const iNITIAL_STACK_SIZE = 100

func newStack() *stack {
	return &stack{
		state:  make([]int, 0, iNITIAL_STACK_SIZE),
		attrib: make([]Attrib, 0, iNITIAL_STACK_SIZE),
	}
}

func (s *stack) reset() {
	s.state = s.state[:0]
	s.attrib = s.attrib[:0]
}

func (s *stack) push(state int, a Attrib) {
	s.state = append(s.state, state)
	s.attrib = append(s.attrib, a)
}

func (s *stack) top() int {
	return s.state[len(s.state)-1]
}

func (s *stack) peek(pos int) int {
	return s.state[pos]
}

func (s *stack) topIndex() int {
	return len(s.state) - 1
}

func (s *stack) popN(items int) []Attrib {
	lo, hi := len(s.state)-items, len(s.state)

	attrib := s.attrib[lo:hi]

	s.state = s.state[:lo]
	s.attrib = s.attrib[:lo]

	return attrib
}

func (s *stack) String() string {
	w := new(bytes.Buffer)
	fmt.Fprintf(w, "stack:\n")
	for i, st := range s.state {
		fmt.Fprintf(w, "\t%d: %d , ", i, st)
		if s.attrib[i] == nil {
			fmt.Fprintf(w, "nil")
		} else {
			switch attr := s.attrib[i].(type) {
			case *token.Token:
				fmt.Fprintf(w, "%s", attr.Lit)
			default:
				fmt.Fprintf(w, "%v", attr)
			}
		}
		fmt.Fprintf(w, "\n")
	}
	return w.String()
}

// Parser

type Parser struct {
	stack     *stack
	nextToken *token.Token
	pos       int
}

type Scanner interface {
	Scan() (tok *token.Token)
}

func NewParser() *Parser {
	p := &Parser{stack: newStack()}
	p.Reset()
	return p
}

func (p *Parser) Reset() {
	p.stack.reset()
	p.stack.push(0, nil)
}

// PanicMode implements panic-mode error recovery.
// The stack top and input symbols are updated appropriately so that parsing can
// be resumed normally.
// TODO: Cap the total number of recoveries during a single run to a constant.
func (p *Parser) PanicMode(scanner Scanner) bool {
	resume := false
	// Traverse the stack until a state s with GOTO defined on a particular
	// nonterminal A is found.
	validState := -1
	NTSymbol := ""
	for i := len(p.stack.state) - 1; i >= 0; i-- {
		// For each state, check the GOTO table for presence of a valid
		// nonterminal.
		for k, v := range gotoTab[p.stack.state[i]] {
			if v != -1 {
				validState = p.stack.state[i]
				// k is the index of the nonterminal symbol.
				NTSymbol = productionsTable[k].Id
				break
			}
		}
		if validState != -1 {
			// numPops is the number of pops to be performed.
			numPops := len(p.stack.state) - 1 - i
			p.stack.popN(numPops)
			// Push the parser state GOTO(s, A) on the stack.
			p.stack.push(validState, NTSymbol)
			break
		}
	}

	// TODO: retrieve the follow set of NTSymbol.
	for _, v := range followsets {
		if v.Nonterminal == NTSymbol {
			// Start discarding the input symbols until a symbol s
			// is found which belongs to the follow set of NTSymbol.
			for {
				if _, ok := v.Terminals[string(p.nextToken.Lit)]; ok {
					// Valid input symbol found.
					fmt.Println(p.nextToken.Lit)
					resume = true
					break
				} else {
					// Discard the input symbol.
					tok := scanner.Scan().Lit
					if len(tok) == 0 {
						// No more input symbols left.
						break
					}
				}
			}
			break
		}
	}
	return resume
}

func (p *Parser) Error(err error, scanner Scanner) (recovered bool, errorAttrib *parseError.Error) {
	errorAttrib = &parseError.Error{
		Err:            err,
		ErrorToken:     p.nextToken,
		ErrorSymbols:   p.popNonRecoveryStates(),
		ExpectedTokens: make([]string, 0, 8),
	}
	for t, action := range actionTab[p.stack.top()].actions {
		if action != nil {
			errorAttrib.ExpectedTokens = append(errorAttrib.ExpectedTokens, token.TokMap.Id(token.Type(t)))
		}
	}

	if action := actionTab[p.stack.top()].actions[token.TokMap.Type("error")]; action != nil {
		p.stack.push(int(action.(shift)), errorAttrib) // action can only be shift
	} else {
		return
	}

	if action := actionTab[p.stack.top()].actions[p.nextToken.Type]; action != nil {
		recovered = true
	}
	for !recovered && p.nextToken.Type != token.EOF {
		p.nextToken = scanner.Scan()
		if action := actionTab[p.stack.top()].actions[p.nextToken.Type]; action != nil {
			recovered = true
		}
	}

	return
}

func (p *Parser) popNonRecoveryStates() (removedAttribs []parseError.ErrorSymbol) {
	if rs, ok := p.firstRecoveryState(); ok {
		errorSymbols := p.stack.popN(p.stack.topIndex() - rs)
		removedAttribs = make([]parseError.ErrorSymbol, len(errorSymbols))
		for i, e := range errorSymbols {
			removedAttribs[i] = e
		}
	} else {
		removedAttribs = []parseError.ErrorSymbol{}
	}
	return
}

// recoveryState points to the highest state on the stack, which can recover
func (p *Parser) firstRecoveryState() (recoveryState int, canRecover bool) {
	recoveryState, canRecover = p.stack.topIndex(), actionTab[p.stack.top()].canRecover
	for recoveryState > 0 && !canRecover {
		recoveryState--
		canRecover = actionTab[p.stack.peek(recoveryState)].canRecover
	}
	return
}

func (p *Parser) newError(err error) error {
	e := &parseError.Error{
		Err:        err,
		StackTop:   p.stack.top(),
		ErrorToken: p.nextToken,
	}
	actRow := actionTab[p.stack.top()]
	for i, t := range actRow.actions {
		if t != nil {
			e.ExpectedTokens = append(e.ExpectedTokens, token.TokMap.Id(token.Type(i)))
		}
	}
	return e
}

func (p *Parser) Parse(scanner Scanner) (res interface{}, err error) {
	p.Reset()
	p.nextToken = scanner.Scan()
	for acc := false; !acc; {
		action := actionTab[p.stack.top()].actions[p.nextToken.Type]
		if action == nil {
			if recovered, errAttrib := p.Error(nil, scanner); !recovered {
				p.nextToken = errAttrib.ErrorToken
				if resume := p.PanicMode(scanner); !resume {
					return nil, p.newError(nil)
				}
			}
			if action = actionTab[p.stack.top()].actions[p.nextToken.Type]; action == nil {
				panic("Error recovery led to invalid action")
			}
		}
		{{- if .Debug }}
		fmt.Printf("S%d %s %s\n", p.stack.top(), token.TokMap.TokenString(p.nextToken), action)
		{{- end }}

		switch act := action.(type) {
		case accept:
			res = p.stack.popN(1)[0]
			acc = true
		case shift:
			p.stack.push(int(act), p.nextToken)
			p.nextToken = scanner.Scan()
		case reduce:
			prod := productionsTable[int(act)]
			attrib, err := prod.ReduceFunc(p.stack.popN(prod.NumSymbols))
			if err != nil {
				if resume := p.PanicMode(scanner); !resume {
					return nil, p.newError(nil)
				}
			} else {
				p.stack.push(gotoTab[p.stack.top()][prod.NTType], attrib)
			}
		default:
			panic("unknown action: " + action.String())
		}
	}
	return res, nil
}
`
