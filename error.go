package ar

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingGlobalHeader indicates that the archive file is invalid because its global
	// header is missing (i.e., because the file is shorter than 8 bytes).
	ErrMissingGlobalHeader = errors.New("ar: missing global header")

	// ErrInvalidGlobalHeader indicates that the archive file is invalid because its global
	// header is malformed (i.e., not the string "!<arch>\n").
	ErrInvalidGlobalHeader = errors.New("ar: invalid global header")
)

// ErrStringTable indicates a problem with the string table in archives that use the GNU variant of
// the file format.
type ErrStringTable struct {
	Err error
}

func (e *ErrStringTable) Error() string {
	return fmt.Sprintf("ar: string table: %s", e.Err)
}

func (e *ErrStringTable) Unwrap() error {
	return e.Err
}

// ErrFileName indicates a problem with the file name in one of the archive's file headers.
type ErrFileName struct {
	Name string
	Err  error
}

func (e *ErrFileName) Error() string {
	return fmt.Sprintf("ar: archive member '%s': %s", e.Name, e.Err)
}

func (e *ErrFileName) Unwrap() error {
	return e.Err
}
