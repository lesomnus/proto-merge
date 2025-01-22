package main_test

import (
	_ "embed"
	"testing"

	. "github.com/lesomnus/proto-merge"
	"github.com/stretchr/testify/require"
)

//go:embed a.proto
var example_a string

//go:embed b.proto
var example_b string

func TestParser(t *testing.T) {
	require := require.New(t)

	v, err := Parser.ParseString("a", example_a)
	require.NoError(err)
	require.NotEmpty(v.Entries)

	i := 0
	test := func(f func(e *Entry)) {
		f(v.Entries[i])
		i++
	}

	test(func(e *Entry) { require.Equal(`"proto3"`, e.Syntax) })
	test(func(e *Entry) { require.Equal(`example.library`, e.Package) })
	test(func(e *Entry) { require.Equal(`"foo.proto"`, e.Import.Package) })
	test(func(e *Entry) { require.Equal(`"baz.proto"`, e.Import.Package) })
	test(func(e *Entry) { require.Equal(`foo.bar`, e.Option.Name) })
	test(func(e *Entry) {
		s := e.Service
		require.NotNil(s)
		require.Equal("FooService", s.Name)
		require.Equal(16, s.Pos.Line)
		require.Equal(1, s.Pos.Column)
		require.Equal(178, s.Pos.Offset)
		require.Equal(20, s.EndPos.Line) // Where the next entry begin.
		require.NotEmpty(s.Entry)

		r := s.Entry[0]
		require.Equal("Add", r.Method.Name)
		require.Equal(17, r.Pos.Line)
		require.Equal(2, r.Pos.Column) // There is an indent.
		require.Equal(200, r.Pos.Offset)
		require.Equal(238, r.EndPos.Offset) // Where the semicolon is.
	})
}
