package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"slices"
	"unicode"

	"github.com/alecthomas/participle/v2/lexer"
)

type Inventory struct {
	Content []byte // original file content.
	Proto   *Proto

	// Sentry which is linked to first Service.
	// List of Services like H-S-E-S-E... where E is right next Entry of previous S.
	Head HorizontalListNode

	Services map[string]*Service // by name.
	Messages map[string]*Message // by name.
}

func NewInventoryFromFile(filename string) (*Inventory, error) {
	v, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return NewInventory(filename, v)
}

func NewInventory(filename string, content []byte) (*Inventory, error) {
	v, err := Parser.ParseBytes(filename, content)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	i := &Inventory{
		Content: content,
		Proto:   v,

		Head: &horizontalListNode{},

		Services: map[string]*Service{},
		Messages: map[string]*Message{},
	}
	last := i.Head
	link := false
	for _, e := range v.Entries {
		if link {
			link = false
			last.SetNext(e)
			last = e
		}
		if v := e.Service; v != nil {
			i.Services[v.Name] = v
			link = true
			last.SetNext(v)
			last = v
		}
		if v := e.Message; v != nil {
			i.Messages[v.Name] = v
		}
	}

	return i, nil
}

func (a *Inventory) MergeOut(b *Inventory, w io.Writer) error {
	// .[][1:] will be printer right after .[][0].
	mss := [][]*Message{}

	n := a.Head.GetNext()
	p := lexer.Position{}
	mv := func(until lexer.Position) {
		v := a.Content[p.Offset:until.Offset]
		v = bytes.TrimRightFunc(v, unicode.IsSpace)
		w.Write(v)
		p = until
	}
	lf := func() {
		w.Write([]byte("\n"))
	}

	// Print services.
	for n != nil {
		s := n.(*Service)
		n = n.GetNext()
		e := n.(*Entry)
		n = n.GetNext()

		s_, ok := b.Services[s.Name]
		if !ok {
			continue
		}
		if len(s.Entry) == 0 || len(s_.Entry) == 0 {
			continue
		}

		// Merge rpcs.
		l := s.Entry[len(s.Entry)-1]
		if p.Offset != 0 {
			lf()
		}
		mv(l.EndPos)
		{
			w.Write([]byte(";\n"))

			l_ := s_.Entry[len(s_.Entry)-1]
			w.Write([]byte("\t")) // TODO: maybe spaces.
			w.Write(b.Content[s_.Entry[0].Pos.Offset:l_.EndPos.Offset])
			_ = e
		}
		mv(s.EndPos)
		lf()

		// Collect rpc messages that are needed to be merged.
		ms := []*Message{nil}
		for _, e := range s.Entry {
			if e.Method == nil {
				continue
			}
			if e.Method.Request != nil {
				m, ok := a.Messages[e.Method.Request.Reference]
				if ok {
					ms[0] = m
				}
			}
			if e.Method.Response != nil {
				m, ok := a.Messages[e.Method.Response.Reference]
				if ok {
					ms[0] = m
				}
			}
		}
		for _, e := range s_.Entry {
			if e.Method == nil {
				continue
			}

			add := func(ref string) error {
				if m, ok := b.Messages[ref]; !ok {
					// Message defined in the other file (e.g. well known messages).
					return nil
				} else if _, ok := a.Messages[m.Name]; ok {
					return fmt.Errorf("duplicated message: %s", m.Name)
				} else {
					ms = append(ms, m)
				}
				return nil
			}

			if err := add(e.Method.Request.Reference); err != nil {
				return err
			}
			if err := add(e.Method.Response.Reference); err != nil {
				return err
			}
		}
		if len(ms) > 1 {
			// There are messages to be merged into.
			mss = append(mss, ms)
		}
	}

	// Print messages.
	slices.SortFunc(mss, func(a, b []*Message) int {
		return a[0].Pos.Offset - b[0].Pos.Offset
	})
	visited := map[string]bool{}
	for _, ms := range mss {
		lf()
		lf()
		if m := ms[0]; p.Offset < m.Pos.Offset {
			// print message in `a` if it is not printed yet.
			mv(m.EndPos)
			lf()
		}
		lf()
		for _, m := range ms[1:] {
			if visited[m.Name] {
				continue
			}
			visited[m.Name] = true

			w.Write(bytes.TrimRightFunc(b.Content[m.Pos.Offset:m.EndPos.Offset], unicode.IsSpace))
			lf()
		}
	}

	w.Write(a.Content[p.Offset:])

	return nil
}
