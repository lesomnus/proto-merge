package main

import (
	_ "embed"
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed a.proto
var aProto []byte

//go:embed b.proto
var bProto []byte

//go:embed c.proto
var cProto []byte

func TestNewInventory_ParsesImports(t *testing.T) {
	inv, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	assert.Contains(t, inv.Imports, `"foo.proto"`)
	assert.Contains(t, inv.Imports, `"baz.proto"`)
	assert.Contains(t, inv.Imports, `"bar.proto"`)
}

func TestNewInventory_ParsesServices(t *testing.T) {
	inv, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	assert.Contains(t, inv.Services, "FooService")
	assert.Contains(t, inv.Services, "AuthorService")
	assert.Contains(t, inv.Services, "BookService")
}

func TestNewInventory_ParsesMessages(t *testing.T) {
	inv, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	assert.Contains(t, inv.Messages, "AuthorAddRequest")
	assert.Contains(t, inv.Messages, "AuthorGetRequest")
	assert.Contains(t, inv.Messages, "BookAddRequest")
	assert.Contains(t, inv.Messages, "BookGetRequest")
}

func TestNewInventory_B_ParsesMessages(t *testing.T) {
	inv, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	assert.Contains(t, inv.Messages, "BookAddRequest")
	assert.Contains(t, inv.Messages, "BookPutRequest")
	assert.Contains(t, inv.Messages, "AuthorPutRequest")
	assert.Contains(t, inv.Messages, "AuthorExtra")
	assert.Contains(t, inv.Messages, "FooState")
}

func TestMerge_AIntoB_MatchesC(t *testing.T) {
	a, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	b, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	var got bytes.Buffer
	err = a.MergeOut(b, &got)
	require.NoError(t, err)

	want := strings.TrimRight(string(cProto), "\n")
	assert.Equal(t, want, strings.TrimRight(got.String(), "\n"))
}

func TestMerge_MergesImports(t *testing.T) {
	a, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	b, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	var got bytes.Buffer
	err = a.MergeOut(b, &got)
	require.NoError(t, err)

	output := got.String()
	// a has bar.proto, baz.proto, foo.proto; b adds qux.proto
	assert.Contains(t, output, `import "bar.proto"`)
	assert.Contains(t, output, `import "baz.proto"`)
	assert.Contains(t, output, `import "foo.proto"`)
	assert.Contains(t, output, `import "qux.proto"`)
}

func TestMerge_MergesServiceMethods(t *testing.T) {
	a, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	b, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	var got bytes.Buffer
	err = a.MergeOut(b, &got)
	require.NoError(t, err)

	output := got.String()
	// AuthorService: a has Add+Get, b adds Put+Pot+List
	assert.Contains(t, output, "rpc Add (AuthorAddRequest) returns (Author)")
	assert.Contains(t, output, "rpc Get (AuthorGetRequest) returns (Author)")
	assert.Contains(t, output, "rpc Put (AuthorPutRequest) returns (Author)")
	assert.Contains(t, output, "rpc List (AuthorListRequest) returns (AuthorListResponse)")

	// BookService: a has Add+Get, b adds Put
	assert.Contains(t, output, "rpc Add (BookAddRequest) returns (Book)")
	assert.Contains(t, output, "rpc Get (BookGetRequest) returns (Book)")
	assert.Contains(t, output, "rpc Put (BookPutRequest) returns (Book)")
}

func TestMerge_MergesMessageFields(t *testing.T) {
	a, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	b, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	var got bytes.Buffer
	err = a.MergeOut(b, &got)
	require.NoError(t, err)

	output := got.String()
	// BookAddRequest: a has id+title, b adds isbn
	assert.Contains(t, output, "optional bytes id = 1")
	assert.Contains(t, output, "string title = 3")
	assert.Contains(t, output, "string isbn = 4")

	// All three fields must appear inside one BookAddRequest block
	start := strings.Index(output, "message BookAddRequest {")
	end := strings.Index(output[start:], "}") + start
	block := output[start : end+1]
	assert.Contains(t, block, "string isbn = 4", "isbn must be inside BookAddRequest")
}

func TestMerge_IncludesMessagesFromB(t *testing.T) {
	a, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	b, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	var got bytes.Buffer
	err = a.MergeOut(b, &got)
	require.NoError(t, err)

	output := got.String()
	// Messages from b that are not in a
	assert.Contains(t, output, "message AuthorPutRequest")
	assert.Contains(t, output, "message AuthorExtra")
	assert.Contains(t, output, "message AuthorListRequest")
	assert.Contains(t, output, "message AuthorListResponse")
	assert.Contains(t, output, "message BookPutRequest")
	assert.Contains(t, output, "enum FooState")
}
