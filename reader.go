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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"
)

// Reader provides read access to an ar archive.
// Call next to skip files.
//
// Example:
//
//     reader := NewReader(f)
//     var buf bytes.Buffer
//     for {
//         _, err := reader.Next()
//         if err == io.EOF {
//             break
//         }
//         if err != nil {
//             t.Errorf(err.Error())
//         }
//         io.Copy(&buf, reader)
//     }
type Reader struct {
	// r is the underlying archive file.
	r *bufio.Reader

	// variant is the variant of the ar file format used by the archive.
	variant Variant

	// nb is the number of bytes in the current data section that remain unread.
	nb int64

	// pad is the number of padding bytes appended to the current data section; it is always either 0
	// or 1, depending on whether the length of the data section is an even or odd number of bytes
	// respectively.
	pad int64

	// stringTable is the archive's string table, the data section of the special "//" file in the GNU
	// variant of the archive format, which stores the names of files that are too long to fit in a
	// file name header field.
	stringTable []byte
}

// NewReader creates a new reader reading from r. It returns an error if the global archive
// header is missing or malformed.
func NewReader(r io.Reader) (*Reader, error) {
	rd := &Reader{
		r:       bufio.NewReader(r),
		variant: BSD,
	}
	// Ensure the global archive header is valid.
	var hdr bytes.Buffer
	if _, err := io.CopyN(&hdr, rd.r, int64(len(GLOBAL_HEADER))); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrMissingGlobalHeader
		}
		return nil, fmt.Errorf("ar: %w", err)
	}
	if string(hdr.Bytes()) != GLOBAL_HEADER {
		return nil, ErrInvalidGlobalHeader
	}
	// Peek at the file name in the archive's first header to determine whether the archive contains a
	// symbol table and identify the file format variant in use. File names in the GNU variant either
	// begin with "/" (special files, file names >= 16 bytes) or end with "/" (file names < 16 bytes);
	// otherwise, assume the archive uses the BSD variant. (This means that empty archives are
	// identified as using the BSD variant, which may not be true, but the distinction doesn't matter
	// for an empty archive anyway.)
	b, err := rd.r.Peek(16)
	if err == nil { // Don't worry about I/O errors here; report them when the caller calls Next.
		firstFile := rd.string(b)
		if len(firstFile) > 0 && (firstFile[0] == '/' || firstFile[len(firstFile)-1] == '/') {
			rd.variant = GNU
		}
	}
	return rd, nil
}

func (rd *Reader) string(b []byte) string {
	i := len(b) - 1
	for i > 0 && b[i] == 32 {
		i--
	}

	return string(b[0 : i+1])
}

func (rd *Reader) numeric(b []byte) int64 {
	i := len(b) - 1
	for i > 0 && b[i] == 32 {
		i--
	}

	n, _ := strconv.ParseInt(string(b[0:i+1]), 10, 64)

	return n
}

func (rd *Reader) octal(b []byte) int64 {
	i := len(b) - 1
	for i > 0 && b[i] == 32 {
		i--
	}

	n, _ := strconv.ParseInt(string(b[0:i+1]), 8, 64)

	return n
}

func (rd *Reader) skipUnread() error {
	skip := rd.nb + rd.pad
	rd.nb, rd.pad = 0, 0
	_, err := io.CopyN(ioutil.Discard, rd.r, skip)
	return err
}

// Next skips to the next file in the archive file.
// Returns a Header which contains the metadata about the
// file in the archive. io.EOF is returned at the end of the input.
func (rd *Reader) Next() (*Header, error) {
	err := rd.skipUnread()
	if err != nil {
		return nil, err
	}

	headerBuf := make([]byte, HEADER_BYTE_SIZE)
	if _, err := io.ReadFull(rd.r, headerBuf); err != nil {
		return nil, err
	}

	s := slicer(headerBuf)
	header := &Header{}
	header.Name = rd.string(s.next(16))
	header.ModTime = time.Unix(rd.numeric(s.next(12)), 0)
	header.Uid = int(rd.numeric(s.next(6)))
	header.Gid = int(rd.numeric(s.next(6)))
	header.Mode = rd.octal(s.next(8))
	header.Size = rd.numeric(s.next(10))

	rd.nb = int64(header.Size)
	if header.Size%2 == 1 {
		rd.pad = 1
	} else {
		rd.pad = 0
	}

	switch rd.variant {
	case GNU:
		switch header.Name {
		// The special file name "/" indicates that the data section contains a symbol table.
		case "/":
			// The symbol table should be invisible to the caller - skip over it.
			return rd.Next()
		// The special file name "//" indicates that the data section contains a string table. The string
		// table contains the names of files in the archive that are >= 15 bytes long, delimited with
		// newlines. Store it, so we can resolve long file names when we encounter them later.
		case "//":
			if rd.stringTable != nil {
				return nil, &ErrStringTable{Err: errors.New("archive contains multiple string tables")}
			}
			buf := make([]byte, rd.nb)
			_, err := rd.Read(buf)
			if err != nil {
				return nil, &ErrStringTable{Err: err}
			}
			rd.stringTable = buf
			// The string table should be invisible to the caller - return the header for the first real file
			// in the archive.
			return rd.Next()
		}
		if err := rd.parseGNUFileName(header); err != nil {
			return nil, err
		}
	case BSD:
		// The special file name "__.SYMDEF" indicates that the data section contains a symbol table.
		if header.Name == "__.SYMDEF" {
			// The symbol table should be invisible to the caller - skip over it.
			return rd.Next()
		if err := rd.parseBSDFileName(header); err != nil {
			return nil, err
		}
	}

	// The file name has now been resolved; make sure it doesn't contain any illegal characters.
	if strings.Contains(header.Name, "/") {
		return nil, &ErrFileName{
			Name: header.Name,
			Err:  errors.New("file name contains illegal '/'"),
		}
	}

	return header, nil
}

func (rd *Reader) parseGNUFileName(header *Header) error {
	if len(header.Name) == 0 {
		return &ErrFileName{
			Name: header.Name,
			Err:  errors.New("zero-length file name"),
		}
	}
	// A file name conisting of "/" followed by an integer indicates that this file has a long name
	// that is stored in the archive's string table. The integer is the byte offset of the real file
	// name in the string table.
	if header.Name[0] == '/' {
		if rd.stringTable == nil {
			return &ErrFileName{
				Name: header.Name,
				Err:  errors.New("missing string table"),
			}
		}
		start, err := strconv.Atoi(header.Name[1:])
		if err != nil || start > len(rd.stringTable) {
			return &ErrFileName{
				Name: header.Name,
				Err:  errors.New("invalid string table offset"),
			}
		}
		tableEntry := rd.stringTable[start:]
		end := bytes.IndexByte(tableEntry, '\n')
		if end == -1 {
			return &ErrStringTable{Err: errors.New("missing trailing newline")}
		}
		header.Name = string(tableEntry[:end])
	}
	// GNU ar appends "/" to all file names, regardless of where they are stored.
	if header.Name[len(header.Name)-1] != '/' {
		return &ErrFileName{
			Name: header.Name,
			Err:  errors.New("file name is missing trailing '/'"),
		}
	}
	header.Name = strings.TrimRight(header.Name, "/")
	return nil
}

func (rd *Reader) parseBSDFileName(header *Header) error {
	// A file name consisting of "#1/" followed by an integer indicates that this file has a long name
	// that is prepended to the file's data section. The integer is the length of the prepended data.
	if strings.HasPrefix(header.Name, "#1/") {
		length, err := strconv.Atoi(header.Name[3:])
		if err != nil {
			return &ErrFileName{
				Name: header.Name,
				Err:  errors.New("invalid long file name length"),
			}
		}
		header.Size -= int64(length)
		b := make([]byte, length)
		if _, err := rd.Read(b); err != nil {
			return &ErrFileName{
				Name: header.Name,
				Err:  err,
			}
		}
		// Some implementations (e.g. llvm-ar) append an indeterminate number of trailing nulls to the
		// prepended data, which should be stripped.
		header.Name = string(bytes.TrimRight(b, "\x00"))
	}
	return nil
}

// Read reads data from the current entry in the archive.
func (rd *Reader) Read(b []byte) (n int, err error) {
	if rd.nb == 0 {
		return 0, io.EOF
	}
	if int64(len(b)) > rd.nb {
		b = b[0:rd.nb]
	}
	n, err = rd.r.Read(b)
	rd.nb -= int64(n)

	return
}
