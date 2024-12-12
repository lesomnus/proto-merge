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
	test(func(e *Entry) { require.Equal(`foo.bar`, e.Option.Name) })
	test(func(e *Entry) {
		s := e.Service
		require.NotNil(s)
		require.Equal("AuthorService", s.Name)
		require.Equal(14, s.Pos.Line)
		require.Equal(1, s.Pos.Column)
		require.Equal(155, s.Pos.Offset)
		require.Equal(22, s.EndPos.Line) // Where the next entry begin.
		require.NotEmpty(s.Entry)

		r := s.Entry[len(s.Entry)-1]
		require.Equal("Get", r.Method.Name)
		require.Equal(18, r.Pos.Line)
		require.Equal(2, r.Pos.Column) // There is an indent.
		require.Equal(308, r.Pos.Offset)
		require.Equal(351, r.EndPos.Offset) // Where the semicolon is.
	})
}
