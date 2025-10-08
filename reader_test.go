package ar

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadHeader(t *testing.T) {
	f, err := os.Open("./test_data/hello.a")
	require.NoError(t, err)
	defer f.Close()

	reader, err := NewReader(f)
	require.NoError(t, err)
	header, err := reader.Next()
	require.NoError(t, err)

	assert.Equal(t, "hello.txt", header.Name)
	assert.Equal(t, time.Unix(1361157466, 0), header.ModTime)
	assert.Equal(t, 501, header.Uid)
	assert.Equal(t, 20, header.Gid)
	assert.Equal(t, int64(0100644), header.Mode)
}

func TestLongFilenames(t *testing.T) {
	for _, tc := range []struct {
		Description string
		Variant     Variant
		ArchivePath string
	}{
		{"BSD format", BSD, "./test_data/long_filenames_bsd.a"},
		{"GNU format", GNU, "./test_data/long_filenames_gnu.a"},
	} {
		t.Run(tc.Description, func(t *testing.T) {
			f, err := os.Open(tc.ArchivePath)
			require.NoError(t, err)
			defer f.Close()
			reader, err := NewReader(f)
			require.NoError(t, err)
			assert.Equal(t, tc.Variant, reader.Variant())
			for i := 1; i <= 20; i++ {
				t.Run("File "+strconv.Itoa(i), func(t *testing.T) {
					hdr, err := reader.Next()
					require.NoError(t, err)
					var buf bytes.Buffer
					assert.Equal(t, fmt.Sprintf("%d%s", i, strings.Repeat("x", i - len(strconv.Itoa(i)))), hdr.Name)
					io.Copy(&buf, reader)
					expected := []byte(fmt.Sprintf("The name of this file contains %d character(s).\n", i))
					assert.Equal(t, hdr.Size, int64(len(expected)))
					actual := buf.Bytes()
					assert.Equal(t, expected, actual)
				})
			}
			hdr, err := reader.Next()
			assert.Nil(t, hdr, "No files left to read")
			assert.ErrorIs(t, err, io.EOF)
		})
	}
}
