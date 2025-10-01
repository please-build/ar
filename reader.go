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
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrMissingGlobalHeader indicates that the archive file is invalid because its global
	// header is missing (i.e., because the file is shorter than 8 bytes).
	ErrMissingGlobalHeader = errors.New("ar: missing global header")

	// ErrInvalidGlobalHeader indicates that the archive file is invalid because its global
	// header is malformed (i.e., not the string "!<arch>\n").
	ErrInvalidGlobalHeader = errors.New("ar: invalid global header")

	// ErrManyStringTables indicates that the archive file contains more than one "//" file
	// denoting the archive's string table. A GNU ar-compatible archive file must contain no
	// more than one string table.
	ErrManyStringTables = errors.New("ar: more than one string table in a GNU archive")

	// ErrStringTableNoLF indicates that the archive's string table does not contain a
	// trailing newline, so its final long file name is unterminated.
	ErrStringTableNoLF = errors.New("ar: string table not newline-terminated")
)

// Reader provides read access to an ar archive.
// Call next to skip files.
//
// N.B. This understands the GNU-style long file entries and will transparently decode them.
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
	// r is the io.Reader of the underlying archive file.
	r io.Reader

	// nb is the number of bytes in the current data section that remain unread.
	nb int64

	// pad is the number of padding bytes appended to the current data section;
	// it is always either 0 or 1, depending on whether the length of the data
	// section is an even or odd number of bytes respectively.
	pad int64

	// stringTable is the archive's string table, the data section of the special
	// "//" file in GNU-format archives, which stores the names of files that are
	// too long to fit in a file name header field.
	stringTable []byte
}

// NewReader creates a new reader reading from r. It returns an error if the global archive
// header is missing or malformed.
func NewReader(r io.Reader) (*Reader, error) {
	var hdr bytes.Buffer
	if _, err := io.CopyN(&hdr, r, int64(len(GLOBAL_HEADER))); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrMissingGlobalHeader
		}
		return nil, fmt.Errorf("ar: %w", err)
	}
	if string(hdr.Bytes()) != GLOBAL_HEADER {
		return nil, ErrInvalidGlobalHeader
	}
	return &Reader{r: r}, nil
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
	if seeker, ok := rd.r.(io.Seeker); ok {
		_, err := seeker.Seek(skip, os.SEEK_CUR)
		return err
	}

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
	header := &Header{
		Name:    rd.string(s.next(16)),
		ModTime: time.Unix(rd.numeric(s.next(12)), 0),
		Uid:     int(rd.numeric(s.next(6))),
		Gid:     int(rd.numeric(s.next(6))),
		Mode:    rd.octal(s.next(8)),
		Size:    rd.numeric(s.next(10)),
	}

	rd.nb = int64(header.Size)
	if header.Size%2 == 1 {
		rd.pad = 1
	} else {
		rd.pad = 0
	}

	// The ar format only supports file names of up to 16 bytes to be stored in the file name header
	// field. Various archivers have adopted different conventions for storing files with longer
	// names:
	// - GNU ar replaces the file name in the header with the character "/" followed by an integer. The
	//   integer is a byte offset into the archive's symbol table - by convention the archive's first
	//   file (or second, if a symbol table is also present) - that contains the archive's long file
	//   names delimited by newlines.
	if header.Name == "//" {
		if rd.stringTable != nil {
			return nil, ErrManyStringTables
		}
		buf := make([]byte, rd.nb)
		_, err := rd.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("ar: read GNU long file name data: %w", err)
		}
		rd.stringTable = buf
		// The string table should be invisible to the caller - return the header for the first real file
		// in the archive.
		return rd.Next()
	} else if header.Name[0] == '/' {
		start, err := strconv.Atoi(header.Name[1:])
		if err != nil {
			return nil, fmt.Errorf("ar: invalid GNU long file name offset '%s'", header.Name[1:])
		}
		fileName := rd.stringTable[start:]
		end := bytes.IndexByte(fileName, '\n')
		if end == -1 {
			return nil, ErrStringTableNoLF
		}
		fileName = bytes.TrimRight(fileName[:end], "/")
		header.Name = string(fileName)
	// - BSD ar replaces the file name in the header with the string "#1/" followed by an integer. The
	//   file name section that would otherwise appear in the header (i.e., including the right padding
	//   with nulls or spaces) is prepended to the data section; the integer represents the length of
	//   this prepended section. (llvm-ar appears to do this for all files, even ones whose name is
	//   shorter than 16 bytes.)
	} else if strings.HasPrefix(header.Name, "#1/") {
		length, err := strconv.Atoi(header.Name[3:])
		if err != nil {
			return nil, err
		}
		header.Size -= int64(length)
		b := make([]byte, length)
		if _, err := rd.Read(b); err != nil {
			return nil, err
		}
		header.Name = string(bytes.TrimRight(b, " \x00"))
	// Otherwise, the file name fits in the standard header field.
	} else {
		// GNU ar appends "/" to file names stored in the header field. We don't necessarily know whether
		// this archive is in GNU or BSD format unless we've encountered a file with a long name before
		// this one, but on the basis that regular file names can't legally end with "/" in Unix,
		// unconditionally strip any trailing "/" character from the file name in the header. At worst,
		// this will be a no-op for archives generated by BSD ar.
		header.Name = strings.TrimRight(header.Name, "/")
	}

	return header, nil
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
