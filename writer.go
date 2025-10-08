package ar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

var (
	ErrWriteTooLong = errors.New("ar: write too long")

	// Epoch is the Unix epoch, 00:00:00 UTC on 1970-01-01.
	Epoch = time.Unix(0, 0)
)

// Writer provides sequential writing of an ar archive.
// An ar archive is sequence of header file pairs
// Call WriteHeader to begin writing a new file, then call Write to supply the file's data
//
// Example:
// archive := ar.NewWriter(writer)
// header := new(ar.Header)
// header.Size = 15 // bytes
// if err := archive.WriteHeader(header); err != nil {
// 	return err
// }
// io.Copy(archive, data)
type Writer struct {
	// w is the underlying io.Writer to which the archive file is written.
	w io.Writer

	// variant is the variant of the ar file format used by the archive.
	variant Variant

	// closed is true if Close has been called on this Writer, or false if it has not.
	closed bool

	// wroteHeader is true if the archive header has been written to the underlying io.Writer, or
	// false if it has not yet.
	wroteHeader bool

	// wroteStringTable is true if the string table has been written to the underlying io.Writer,
	// or false if it has not yet.
	//
	// This field's value is only meaningful when variant is GNU - BSD-style archives do not
	// contain a string table.
	wroteStringTable bool

	// nb is the number of bytes that have been written via Write since the most recent call to
	// WriteHeader.
	nb int64

	// stringTable is the archive's string table, which maps archive members' file names that are
	// over 15 bytes long to the byte offset of that file name within the string table.
	//
	// This field's value is only meaningful when variant is GNU - BSD-style archives do not
	// contain a string table.
	stringTable map[string]int
}

// NewWriter creates a new Writer that writes an ar archive of the given variant to an underlying
// io.Writer.
func NewWriter(w io.Writer, variant Variant) *Writer {
	return &Writer{
		w:           w,
		variant:     variant,
		stringTable: map[string]int{},
	}
}

func (aw *Writer) numeric(b []byte, x int64) {
	s := strconv.FormatInt(x, 10)
	for len(s) < len(b) {
		s = s + " "
	}
	copy(b, []byte(s))
}

func (aw *Writer) octal(b []byte, x int64) {
	s := "100" + strconv.FormatInt(x, 8)
	for len(s) < len(b) {
		s = s + " "
	}
	copy(b, []byte(s))
}

func (aw *Writer) string(b []byte, str string) {
	s := str
	for len(s) < len(b) {
		s = s + " "
	}
	copy(b, []byte(s))
}

func (aw *Writer) write(p []byte) (int, error) {
	if aw.closed {
		return 0, errors.New("ar: write to closed writer")
	}
	aw.writeHeader()
	return aw.w.Write(p)
}

// Close finishes writing the archive, ensuring that a valid archive header has been written even if
// the archive contains no files. It does not close the underlying io.Writer.
func (aw *Writer) Close() error {
	if aw.closed {
		return errors.New("ar: writer closed twice")
	}
	aw.writeHeader()
	aw.closed = true
	return nil
}

// Writes to the current entry in the ar archive
// Returns ErrWriteTooLong if more than header.Size
// bytes are written after a call to WriteHeader
func (aw *Writer) Write(b []byte) (n int, err error) {
	if int64(len(b)) > aw.nb {
		b = b[0:aw.nb]
		err = ErrWriteTooLong
	}
	n, werr := aw.write(b)
	aw.nb -= int64(n)
	if werr != nil {
		return n, werr
	}

	if len(b)%2 == 1 { // data size must be aligned to an even byte
		if _, err := aw.write([]byte{'\n'}); err != nil {
			// Return n although we actually wrote n+1 bytes.
			// This is to make io.Copy() to work correctly.
			return n, err
		}
	}

	return
}

// writeHeader writes the ar header to the underlying io.Writer. This must only happen once, and must
// be the first write operation on the io.Writer.
func (aw *Writer) writeHeader() error {
	if aw.wroteHeader {
		return nil
	}
	aw.wroteHeader = true
	_, err := aw.write([]byte(GLOBAL_HEADER))
	if err != nil {
		return fmt.Errorf("ar: write archive header: %w", err)
	}
	return nil
}

// WriteStringTable writes a string table for GNU-format archives.
//
// The string table is a list of file names of archive members that are more than 15 bytes long
// (although file names of 15 bytes or less may also be stored). It is the first member of a GNU-format
// archive (or the second, if the archive also contains a symbol table), which means that this function
// must be called before the first call to WriteHeader if the archive is to contain members with a file
// name length of more than 15 bytes.
//
// The BSD variant of the ar file format has no concept of string tables, and this function has no
// effect if this Writer is writing a BSD-format archive.
func (aw *Writer) WriteStringTable(filenames []string) error {
	if aw.variant != GNU {
		return errors.New("ar: wrote string table for BSD-variant archive")
	}
	if aw.wroteStringTable {
		return errors.New("ar: wrote string table twice")
	}
	aw.wroteStringTable = true
	var data []byte
	for _, filename := range filenames {
		aw.stringTable[filename] = len(data)
		data = append(data, []byte(filename)...)
		data = append(data, '/')
		data = append(data, '\n')
	}
	if len(data) == 0 {
		return nil
	}
	// need at least one long filename
	if err := aw.WriteHeader(&Header{Name: "//", Mode: 0420, Size: int64(len(data))}); err != nil {
		return err
	}
	_, err := io.Copy(aw, bytes.NewReader(data))
	return err
}

// Writes the header to the underlying writer and prepares
// to receive the file payload
func (aw *Writer) WriteHeader(hdr *Header) error {
	aw.nb = int64(hdr.Size)
	header := make([]byte, HEADER_BYTE_SIZE)
	s := slicer(header)

	if len(hdr.Name) == 0 {
		return errors.New("ar: empty file name")
	}

	switch aw.variant {
	case GNU:
		// "/" is always appended to GNU-variant file names, which means that any file names over 15 bytes
		// long must be stored in the string table, even though there's 16 bytes of space in the header
		// for the file name.
		if len(hdr.Name) > 15 {
			if !aw.wroteStringTable {
				return errors.New("ar: missing string table")
			}
			offset, present := aw.stringTable[hdr.Name]
			if !present {
				return fmt.Errorf("ar: missing string table entry for file name '%s'", hdr.Name)
			}
			aw.string(s.next(16), "/"+strconv.Itoa(offset))
		} else {
			aw.string(s.next(16), hdr.Name)
		}
	case BSD:
		// In the BSD variant of the ar format, file names that won't fit in the file name header are
		// prepended to the data section; the length of the file name is inserted into the field in its
		// place, so the reader knows how much to read from the front of the data section. Because
		// BSD-variant file name header fields have no trailing character delimiting the file name from
		// the spaces in the field padding, also do this for file names containing spaces (even when the
		// spaces occur before the end of the file name, in case the reader reads the file name header
		// byte by byte and stops when it encounters the first space).
		if len(hdr.Name) > 16 || strings.ContainsRune(hdr.Name, ' ') {
			// The ar file format requires data sections to be an even number of bytes long. Since the real
			// file name is being prepended to the data section, pad it with one null byte if it has an odd
			// length (the padding byte will be ignored when read). Write will take care of the padding for
			// the real data section, ensuring that the data section has an even length overall.
			if len(hdr.Name)%2 == 1 {
				hdr.Name += "\x00"
			}
			aw.string(s.next(16), "#1/"+strconv.Itoa(len(hdr.Name)))
			aw.nb += int64(len(hdr.Name))
			hdr.Size += int64(len(hdr.Name))
		} else {
			aw.string(s.next(16), hdr.Name)
		}
	default:
		// This should be unreachable.
		return errors.New("ar: unsupported variant")
	}
	// Modification times before the Unix epoch cannot meaningfully be represented in ar headers, which
	// store times as stringified Unix times - ensure the modification time is at least the epoch.
	if hdr.ModTime.Before(Epoch) {
		hdr.ModTime = Epoch
	}
	aw.numeric(s.next(12), hdr.ModTime.Unix())
	aw.numeric(s.next(6), int64(hdr.Uid))
	aw.numeric(s.next(6), int64(hdr.Gid))
	aw.octal(s.next(8), hdr.Mode)
	aw.numeric(s.next(10), hdr.Size)
	aw.string(s.next(2), "`\n")

	_, err := aw.write(header)
	if err != nil {
		return fmt.Errorf("ar: write member header: %w", err)
	}

	if aw.variant == BSD && len(hdr.Name) > 16 {
		if _, err = aw.Write([]byte(hdr.Name)); err != nil {
			return fmt.Errorf("ar: write BSD-variant file name: %w", err)
		}
	}

	return nil
}
