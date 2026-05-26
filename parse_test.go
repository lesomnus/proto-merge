package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_Syntax(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"package only", `package sample;`},
		{"syntax proto2", `syntax = "proto2";`},
		{"syntax proto3", `syntax = "proto3";`},
		{"edition 2023", `edition = "2023";`},
		{"edition and package", `edition = "2023"; package sample;`},

		{"message without edition", `message Foo { uint64 id = 1; }`},
		{"message with optional field", `message Foo { optional string name = 1; }`},
		{"message with required field", `message Foo { required bytes data = 1; }`},
		{"message with repeated field", `message Foo { repeated string tags = 1; }`},
		{"message with map field", `message Foo { map<string, string> labels = 1; }`},
		{"message with map field (message value)", `message Foo { map<uint64, Foo> items = 1; }`},
		{"message with oneof", `message Foo { oneof key { uint64 id = 1; string slug = 2; } }`},
		{"message with field option", `message Foo { uint64 id = 1 [deprecated = true]; }`},
		{"message with multiple field options", `message Foo { uint64 id = 1 [deprecated = true, json_name = "ID"]; }`},
		{"message with features option", `message Foo { uint64 id = 1 [features.field_presence = IMPLICIT]; }`},
		{"message with nested message", `message Foo { message Bar { string name = 1; } uint64 id = 2; }`},
		{"message with nested enum", `message Foo { enum Kind { UNKNOWN = 0; } Kind kind = 1; }`},
		{"message with reserved range", `message Foo { reserved 2 to 15; uint64 id = 1; }`},
		{"message with reserved name", `message Foo { reserved "old_field"; uint64 id = 1; }`},
		{"multiple messages", `message Foo { uint64 id = 1; } message Bar { string name = 1; }`},

		{"enum", `enum Status { UNKNOWN = 0; ACTIVE = 1; INACTIVE = 2; }`},
		{"enum with option", `enum Status { option allow_alias = true; UNKNOWN = 0; ACTIVE = 1; ALIAS = 1; }`},
		{"enum with value option", `enum Status { UNKNOWN = 0 [deprecated = true]; ACTIVE = 1; }`},
		{"enum negative value", `enum Kind { UNKNOWN = 0; LEGACY = -1; }`},

		{"import", `import "google/protobuf/empty.proto";`},
		{"multiple imports", `import "google/protobuf/empty.proto"; import "google/protobuf/timestamp.proto";`},

		{"service", `service FooService { rpc Get(GetRequest) returns (GetResponse); }`},
		{"service with streaming request", `service FooService { rpc Watch(stream WatchRequest) returns (WatchResponse); }`},
		{"service with streaming response", `service FooService { rpc List(ListRequest) returns (stream ListResponse); }`},
		{"service with method option", `service FooService { rpc Get(GetRequest) returns (GetResponse) { option deprecated = true; } }`},

		{"option", `option go_package = "example.com/foo";`},

		{"extend", `extend google.protobuf.FieldOptions { bool my_option = 50000; }`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewInventory("test.proto", []byte(tc.input))
			require.NoError(t, err)
		})
	}
}

func TestParse_PopulatesMessages(t *testing.T) {
	inv, err := NewInventory("test.proto", []byte(`
message Foo { uint64 id = 1; }
message Bar { string name = 1; }
`))
	require.NoError(t, err)

	assert.Contains(t, inv.Messages, "Foo")
	assert.Contains(t, inv.Messages, "Bar")
}

func TestParse_PopulatesEnums(t *testing.T) {
	inv, err := NewInventory("test.proto", []byte(`
enum Status { UNKNOWN = 0; ACTIVE = 1; }
`))
	require.NoError(t, err)

	// Enums are stored in Messages map by name.
	assert.Contains(t, inv.Messages, "Status")
}

func TestParse_PopulatesServices(t *testing.T) {
	inv, err := NewInventory("test.proto", []byte(`
service FooService { rpc Get(GetRequest) returns (GetResponse); }
`))
	require.NoError(t, err)

	assert.Contains(t, inv.Services, "FooService")
}

func TestParse_PopulatesImports(t *testing.T) {
	inv, err := NewInventory("test.proto", []byte(`
import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";
`))
	require.NoError(t, err)

	assert.Contains(t, inv.Imports, `"google/protobuf/empty.proto"`)
	assert.Contains(t, inv.Imports, `"google/protobuf/timestamp.proto"`)
}

func TestParse_Testdata(t *testing.T) {
	err := filepath.WalkDir("testdata", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".proto" {
			return nil
		}
		t.Run(path, func(t *testing.T) {
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			_, err = NewInventory(path, content)
			assert.NoError(t, err)
		})
		return nil
	})
	require.NoError(t, err)
}
