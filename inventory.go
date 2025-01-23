package main

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/alecthomas/participle/v2/lexer"
)

type Inventory struct {
	Content []byte // original file content.
	Proto   *Proto

	Imports  map[string]*Import  // by package.
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

		Imports:  map[string]*Import{},
		Services: map[string]*Service{},
		Messages: map[string]*Message{},
	}
	for _, e := range v.Entries {
		switch {
		case e.Import != nil:
			i.Imports[e.Import.Package] = e.Import
		case e.Service != nil:
			i.Services[e.Service.Name] = e.Service
		case e.Message != nil:
			i.Messages[e.Message.Name] = e.Message
		}
	}

	return i, nil
}

func (a *Inventory) MergeOut(b *Inventory, w io.Writer) error {
	// First two messages for each array are from the `a` and rest are from the `b`.
	msgs := [][]*Message{{nil, nil}}

	last := lexer.Position{}
	mv := func(until lexer.Position) {
		if last.Offset > until.Offset {
			return
		}

		v := a.Content[last.Offset:until.Offset]
		if i := bytes.LastIndexFunc(v, func(r rune) bool {
			return !unicode.IsSpace(r)
		}); i > 0 {
			until.Offset -= (len(v) - i - 1)
			v = a.Content[last.Offset:until.Offset]
		}

		w.Write(v)
		last = until
	}
	lf := func() { w.Write([]byte("\n")) }
	tab := func() { w.Write([]byte("\t")) }

	is_import_hit := false
	for _, e := range a.Proto.Entries {
		switch {
		case e.Import != nil && !is_import_hit:
			is_import_hit = true
			mv(e.Import.Pos)
			lf()

		case e.Option != nil:
			// Merge imports.
			imports := maps.Clone(b.Imports)
			for n, v := range a.Imports {
				imports[n] = v
			}

			// Sort by package.
			vs := slices.Collect(maps.Values(imports))
			slices.SortFunc(vs, func(a *Import, b *Import) int {
				return strings.Compare(a.Package, b.Package)
			})

			// Skip move
			last = e.Pos
			lf()

			for _, v := range vs {
				w.Write([]byte(fmt.Sprintf("import %s;\n", v.Package)))
			}
			lf()

		case e.Service != nil:
			// Merge service.
			v, ok := b.Services[e.Service.Name]
			if !ok {
				continue
			}

			// Print entries from `a`.
			u := e.Service.Entry[len(e.Service.Entry)-1]
			mv(u.EndPos)
			lf()
			tab()

			// Print entries from `b`.
			x := v.Pos
			y := v.Entry[len(v.Entry)-1].EndPos
			for {
				o := x.Offset
				x.Offset++
				if b.Content[o] == '{' {
					break
				}
			}

			w.Write(b.Content[x.Offset:y.Offset])

			// Print service close.
			mv(e.Service.EndPos)

			// Collect messages to be printed.
			for _, se := range e.Service.Entry {
				v := se.Method
				if v == nil {
					continue
				}

				msgs = append(msgs, []*Message{
					a.Messages[v.Request.Reference],
					a.Messages[v.Request.Reference],
				})
			}

			// Collect messages to be merged into.
			ms := msgs[len(msgs)-1]
			for _, e := range v.Entry {
				v := e.Method
				if v == nil {
					continue
				}

				if m := b.Messages[v.Request.Reference]; m != nil {
					for _, m_nested := range m.Entries {
						if m_nested.Field == nil {
							continue
						}
						ms = append(ms, b.Messages[m_nested.Field.Type.Reference])
					}
				}
				ms = append(ms, b.Messages[v.Request.Reference])

				if m := b.Messages[v.Response.Reference]; m != nil {
					for _, m_nested := range m.Entries {
						if m_nested.Field == nil {
							continue
						}
						ms = append(ms, b.Messages[m_nested.Field.Type.Reference])
					}
				}
				ms = append(ms, b.Messages[v.Response.Reference])
			}
			msgs[len(msgs)-1] = ms
		}
	}

	// Print messages
	msgs_written := map[string]bool{}
	for _, ms := range msgs {
		ms_a := ms[:2]
		ms_b := ms[2:]

		for _, m := range ms_a {
			if m == nil {
				continue
			}

			msgs_written[m.Name] = true
			mv(m.EndPos)
		}
		for _, m := range ms_b {
			if m == nil {
				continue
			}
			_, ok := msgs_written[m.Name]
			if ok {
				continue
			}
			msgs_written[m.Name] = true
			w.Write(b.Content[m.Pos.Offset:m.EndPos.Offset])
		}
	}

	// Write out rest of the content.
	w.Write(a.Content[last.Offset:])

	return nil
}
