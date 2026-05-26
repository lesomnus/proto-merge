package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			dir := filepath.Join("testdata", entry.Name())

			a, err := NewInventoryFromFile(filepath.Join(dir, "a.proto"))
			require.NoError(t, err)

			b, err := NewInventoryFromFile(filepath.Join(dir, "b.proto"))
			require.NoError(t, err)

			want, err := os.ReadFile(filepath.Join(dir, "c.proto"))
			require.NoError(t, err)

			var got bytes.Buffer
			err = a.MergeOut(b, &got)
			require.NoError(t, err)

			assert.Equal(t,
				strings.TrimRight(string(want), "\n"),
				strings.TrimRight(got.String(), "\n"),
			)
		})
	}
}
