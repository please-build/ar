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
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReadHeader(t *testing.T) {
	f, err := os.Open("./fixtures/hello.a")
	defer f.Close()

	if err != nil {
		t.Errorf(err.Error())
	}
	reader, err := NewReader(f)
	if err != nil {
		t.Errorf(err.Error())
	}
	header, err := reader.Next()
	if err != nil {
		t.Errorf(err.Error())
	}

	expectedName := "hello.txt"
	if header.Name != expectedName {
		t.Errorf("Header name should be %s but is %s", expectedName, header.Name)
	}
	expectedModTime := time.Unix(1361157466, 0)
	if header.ModTime != expectedModTime {
		t.Errorf("ModTime should be %s but is %s", expectedModTime, header.ModTime)
	}
	expectedUid := 501
	if header.Uid != expectedUid {
		t.Errorf("Uid should be %d but is %d", expectedUid, header.Uid)
	}
	expectedGid := 20
	if header.Gid != expectedGid {
		t.Errorf("Gid should be %d but is %d", expectedGid, header.Gid)
	}
	expectedMode := int64(0100644)
	if header.Mode != expectedMode {
		t.Errorf("Mode should be %d but is %d", expectedMode, header.Mode)
	}
}

func TestReadBody(t *testing.T) {
	f, err := os.Open("./fixtures/hello.a")
	defer f.Close()

	if err != nil {
		t.Errorf(err.Error())
	}
	reader, err := NewReader(f)
	if err != nil {
		t.Errorf(err.Error())
	}
	_, err = reader.Next()
	if err != nil && err != io.EOF {
		t.Errorf(err.Error())
	}
	var buf bytes.Buffer
	io.Copy(&buf, reader)

	expected := []byte("Hello world!\n")
	actual := buf.Bytes()
	if !bytes.Equal(actual, expected) {
		t.Errorf("Data value should be %s but is %s", expected, actual)
	}
}

func TestReadMulti(t *testing.T) {
	f, err := os.Open("./fixtures/multi_archive.a")
	defer f.Close()

	if err != nil {
		t.Errorf(err.Error())
	}
	reader, err := NewReader(f)
	if err != nil {
		t.Errorf(err.Error())
	}
	var buf bytes.Buffer
	for {
		_, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Errorf(err.Error())
		}
		io.Copy(&buf, reader)
	}
	expected := []byte("Hello world!\nI love lamp.\n")
	actual := buf.Bytes()
	if !bytes.Equal(expected, actual) {
		t.Errorf("Concatted byte buffer should be %s but is %s", expected, actual)
	}
}

func TestLongFilename(t *testing.T) {
	for _, tc := range []struct {
		Description string
		ArchivePath string
	}{
		{"BSD format", "./fixtures/bsd_long_filename.a"},
		{"GNU format", "./fixtures/gnu_long_filename.a"},
	} {
		t.Run(tc.Description, func(t *testing.T) {
			f, err := os.Open(tc.ArchivePath)
			assert.NoError(t, err)
			defer f.Close()
			reader, err := NewReader(f)
			assert.NoError(t, err)
			var buf bytes.Buffer
			hdr, err := reader.Next()
			assert.NoError(t, err)
			assert.Equal(t, "test_long_filename.txt", hdr.Name)
			assert.EqualValues(t, 33, hdr.Size)
			io.Copy(&buf, reader)
			expected := []byte("test a file with a long filename\n")
			assert.EqualValues(t, hdr.Size, len(expected))
			actual := buf.Bytes()
			assert.Equal(t, expected, actual)
		})
	}
}
