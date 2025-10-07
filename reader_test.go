/*
Copyright (c) 2013 Blake Smith <blakesmith0@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
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
	f, err := os.Open("./fixtures/hello.a")
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
		ArchivePath string
	}{
		{"BSD format", "./fixtures/long_filenames_bsd.a"},
		{"GNU format", "./fixtures/long_filenames_gnu.a"},
	} {
		t.Run(tc.Description, func(t *testing.T) {
			f, err := os.Open(tc.ArchivePath)
			require.NoError(t, err)
			defer f.Close()
			reader, err := NewReader(f)
			require.NoError(t, err)
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
