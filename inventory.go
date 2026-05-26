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
	Messages map[string]Posed    // by name, including enums.
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
		Messages: map[string]Posed{},
	}
	for _, e := range v.Entries {
		switch {
		case e.Import != nil:
			i.Imports[e.Import.Package] = e.Import
		case e.Service != nil:
			i.Services[e.Service.Name] = e.Service
		case e.Message != nil:
			i.Messages[e.Message.Name] = e.Message
		case e.Enum != nil:
			i.Messages[e.Enum.Name] = e.Enum
		}
	}

	return i, nil
}

// detectIndent returns the whitespace prefix of the line at offset in content.
// Falls back to a single tab if the prefix contains non-whitespace characters.
func detectIndent(content []byte, offset int) []byte {
	i := offset - 1
	for i >= 0 && content[i] != '\n' {
		i--
	}
	if i < 0 {
		return []byte("\t")
	}
	indent := content[i+1 : offset]
	for _, b := range indent {
		if b != ' ' && b != '\t' {
			return []byte("\t")
		}
	}
	// Return a copy to avoid aliasing with content when the caller appends.
	return append([]byte{}, indent...)
}

func (a *Inventory) MergeOut(b *Inventory, w io.Writer) error {
	// First two messages for each array are from the `a` and rest are from the `b`.
	msgs := [][]Posed{{nil, nil}}

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

				msgs = append(msgs, []Posed{
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

				ms = append(ms, collectMessagesRecursive(b.Messages, b.Messages[v.Request.Reference])...)
				ms = append(ms, collectMessagesRecursive(b.Messages, b.Messages[v.Response.Reference])...)
			}
			msgs[len(msgs)-1] = ms
		}
	}

	msgs_written := map[string]bool{}

	// merge_or_write_message writes m_msg from a.Content, injecting any extra fields
	// found in b's version of the same message. Falls back to a plain mv if no
	// extra fields exist or the message has already been written.
	merge_or_write_message := func(m_msg *Message) {
		b_posed, has_b_version := b.Messages[m_msg.Ident()]
		b_msg, b_is_msg := b_posed.(*Message)
		if !has_b_version || !b_is_msg {
			mv(m_msg.End())
			return
		}

		// Collect all field names from a (top-level and inside oneofs).
		a_fields := map[string]bool{}
		for _, e := range m_msg.Entries {
			if e.Field != nil {
				a_fields[e.Field.Name] = true
			}
			if e.Oneof != nil {
				for _, oe := range e.Oneof.Entries {
					if oe.Field != nil {
						a_fields[oe.Field.Name] = true
					}
				}
			}
		}

		// Find extra top-level fields and extra fields inside named oneofs from b.
		var extra_top []*MessageEntry
		extra_oneof := map[string][]*OneofEntry{} // oneof name -> extra entries
		for _, e := range b_msg.Entries {
			if e.Field != nil && !a_fields[e.Field.Name] {
				extra_top = append(extra_top, e)
			}
			if e.Oneof != nil {
				for _, oe := range e.Oneof.Entries {
					if oe.Field != nil && !a_fields[oe.Field.Name] {
						extra_oneof[e.Oneof.Name] = append(extra_oneof[e.Oneof.Name], oe)
					}
				}
			}
		}

		if len(extra_top) == 0 && len(extra_oneof) == 0 {
			mv(m_msg.End())
			return
		}

		// Detect field-level indentation from the first entry in the message.
		msg_indent := []byte("\t")
		if len(m_msg.Entries) > 0 {
			msg_indent = detectIndent(a.Content, m_msg.Entries[0].Pos.Offset)
		}

		close_offset := m_msg.EndPos.Offset - 1
		for a.Content[close_offset] != '}' {
			close_offset--
		}

		if last.Offset > close_offset {
			// Already written (duplicate occurrence in ms_a).
			mv(m_msg.End())
			return
		}

		// Inject extra fields into matching oneofs in a.
		for _, e := range m_msg.Entries {
			if e.Oneof == nil {
				continue
			}
			extras, ok := extra_oneof[e.Oneof.Name]
			if !ok {
				continue
			}

			// Detect field-level indentation inside this oneof.
			oneof_indent := append(msg_indent, msg_indent...)
			if len(e.Oneof.Entries) > 0 {
				oneof_indent = detectIndent(a.Content, e.Oneof.Entries[0].Pos.Offset)
			}

			oneof_close := e.Oneof.EndPos.Offset - 1
			for a.Content[oneof_close] != '}' {
				oneof_close--
			}

			oneof_close_pos := e.Oneof.EndPos
			oneof_close_pos.Offset = oneof_close
			mv(oneof_close_pos)

			for _, oe := range extras {
				// ';' is outside OneofEntry grammar, so find it explicitly.
				end := oe.EndPos.Offset
				for end < len(b.Content) && (b.Content[end] == ' ' || b.Content[end] == '\t') {
					end++
				}
				if end < len(b.Content) && b.Content[end] == ';' {
					end++
				}
				content := bytes.TrimRight(b.Content[oe.Pos.Offset:end], " \t\r\n")
				lf()
				w.Write(oneof_indent)
				w.Write(content)
			}
		}

		// Inject extra top-level fields before the message's closing }.
		close_pos := m_msg.EndPos
		close_pos.Offset = close_offset

		mv(close_pos)
		for _, e := range extra_top {
			lf()
			w.Write(msg_indent)
			content := bytes.TrimRight(b.Content[e.Pos.Offset:e.EndPos.Offset], " \t\r\n")
			w.Write(content)
		}
		mv(m_msg.End())
	}

	// Print messages referenced by merged services.
	for _, ms := range msgs {
		ms_a := ms[:2]
		ms_b := ms[2:]

		for _, m := range ms_a {
			if m == nil {
				continue
			}
			msgs_written[m.Ident()] = true
			m_msg, is_msg := m.(*Message)
			if !is_msg {
				mv(m.End())
				continue
			}
			merge_or_write_message(m_msg)
		}
		for _, m := range ms_b {
			if m == nil {
				continue
			}
			if msgs_written[m.Ident()] {
				continue
			}
			msgs_written[m.Ident()] = true
			w.Write(b.Content[m.Begin().Offset:m.End().Offset])
		}
	}

	// Final pass: write remaining a messages not yet handled, merging fields from b.
	for _, e := range a.Proto.Entries {
		if e.Message == nil {
			continue
		}
		m := e.Message
		if m.Begin().Offset < last.Offset {
			continue // already written by the service-based pass above
		}
		if msgs_written[m.Ident()] {
			// Written from a.Content via ms_a; ensure last is advanced past it.
			if m.End().Offset > last.Offset {
				last = m.End()
			}
			continue
		}
		msgs_written[m.Ident()] = true
		merge_or_write_message(m)
	}

	// Write new messages from b that don't exist in a.
	for _, e := range b.Proto.Entries {
		if e.Message == nil || msgs_written[e.Message.Name] {
			continue
		}
		msgs_written[e.Message.Name] = true
		lf()
		lf()
		w.Write(b.Content[e.Message.Begin().Offset:e.Message.End().Offset])
	}

	// Write any remaining a.Content (trailing content after the last message).
	w.Write(a.Content[last.Offset:])

	return nil
}

func collectMessagesRecursive(pool map[string]Posed, m Posed) []Posed {
	vs := []Posed{}
	if m == nil {
		return vs
	}

	vs = append(vs, m)
	m_, ok := m.(*Message)
	if !ok {
		return vs
	}

	for _, v := range m_.Entries {
		switch {
		case v.Enum != nil:
			vs = append(vs, v.Enum)

		case v.Field != nil:
			m := pool[v.Field.Type.Reference]
			if m == nil {
				continue
			}

			vs = append(vs, collectMessagesRecursive(pool, m)...)

		case v.Oneof != nil:
			for _, v_ := range v.Oneof.Entries {
				if v_.Field == nil {
					continue
				}

				m := pool[v_.Field.Type.Reference]
				if m == nil {
					continue
				}

				vs = append(vs, collectMessagesRecursive(pool, m)...)
			}
		}
	}

	return vs
}
