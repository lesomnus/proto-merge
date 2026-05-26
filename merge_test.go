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

	assert.Contains(t, inv.Imports, `"google/protobuf/empty.proto"`)
	assert.Contains(t, inv.Imports, `"hday/cove/blob.proto"`)
}

func TestNewInventory_ParsesServices(t *testing.T) {
	inv, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	assert.Contains(t, inv.Services, "BlobService")
}

func TestNewInventory_ParsesMessages(t *testing.T) {
	inv, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	assert.Contains(t, inv.Messages, "BlobAddRequest")
	assert.Contains(t, inv.Messages, "BlobRef")
	assert.Contains(t, inv.Messages, "BlobSelect")
	assert.Contains(t, inv.Messages, "BlobGetRequest")
	assert.Contains(t, inv.Messages, "BlobPatchRequest")
}

func TestNewInventory_B_ParsesMessages(t *testing.T) {
	inv, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	assert.Contains(t, inv.Messages, "BlobRef")
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

func TestMerge_MergesMessageFields(t *testing.T) {
	a, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	b, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	var got bytes.Buffer
	err = a.MergeOut(b, &got)
	require.NoError(t, err)

	output := got.String()

	// BlobRef: a has oneof key {id, digest}, b adds data as top-level field
	start := strings.Index(output, "message BlobRef {")
	require.NotEqual(t, -1, start, "BlobRef not found in output")
	end := strings.Index(output[start:], "\nmessage ")
	if end == -1 {
		end = len(output) - start
	}
	block := output[start : start+end]

	assert.Contains(t, block, "uint64 id = 1", "id must be inside BlobRef")
	assert.Contains(t, block, "bytes digest = 8", "digest must be inside BlobRef")
	assert.Contains(t, block, "bytes data = 9", "data from b must be merged into BlobRef")
}

func TestMerge_PreservesExistingFields(t *testing.T) {
	a, err := NewInventory("a.proto", aProto)
	require.NoError(t, err)

	b, err := NewInventory("b.proto", bProto)
	require.NoError(t, err)

	var got bytes.Buffer
	err = a.MergeOut(b, &got)
	require.NoError(t, err)

	output := got.String()
	// All original a messages must still be present.
	assert.Contains(t, output, "message BlobAddRequest")
	assert.Contains(t, output, "message BlobSelect")
	assert.Contains(t, output, "message BlobGetRequest")
	assert.Contains(t, output, "message BlobPatchRequest")
}
