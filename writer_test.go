package ar

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalHeaderWrite(t *testing.T) {
	var buf bytes.Buffer
	writer := NewWriter(&buf, GNU)
	err := writer.Close()
	require.NoError(t, err)
	assert.Equal(t, []byte("!<arch>\n"), buf.Bytes())
}

func TestWriteTooLong(t *testing.T) {
	body := "Hello world!\n"

	hdr := new(Header)
	hdr.Name = "hello.txt"
	hdr.Size = 1

	var buf bytes.Buffer
	writer := NewWriter(&buf, GNU)
	writer.WriteHeader(hdr)
	_, err := writer.Write([]byte(body))
	assert.ErrorIs(t, err, ErrWriteTooLong)
}

// TestWriteValidArchive ensures that the archive files written by Writer are capable of being read
// by a range of third-party ar tools. Subsets of the test cases run in different CI environments
// depending on the availability of the third-party ar tools on each runner type.
func TestWriteValidArchive(t *testing.T) {
	// CI=true is always set on GitHub runners.
	// https://docs.github.com/en/actions/reference/workflows-and-actions/variables
	if os.Getenv("CI") != "true" {
		t.Skip("CI-only test")
	}

	fileNames := make([]string, 20)
	longFileNames := make([]string, 0)
	for i := 1; i <= 20; i++ {
		fileName := fmt.Sprintf("%d%s", i, strings.Repeat("x", i - len(strconv.Itoa(i))))
		fileNames[i-1] = fileName
		if len(fileName) > 15 {
			longFileNames = append(longFileNames, fileName)
		}
	}

	for _, tc := range []struct {
		Description string
		ArPath      string
		ArArgs      []string
		Variant     Variant
		Prereq      func(string) bool
	}{
		{
			Description: "GNU ar",
			ArPath:      "/usr/bin/ar",
			ArArgs:      []string{},
			Variant:     GNU,
			Prereq:      func(os string) bool { return os == "Linux" },
		},
		{
			Description: "llvm-ar in GNU mode",
			ArPath:      "/usr/bin/llvm-ar-18",
			ArArgs:      []string{"--format=gnu"},
			Variant:     GNU,
			Prereq:      func(os string) bool { return os == "Linux" },
		},
		{
			Description: "llvm-ar in BSD mode",
			ArPath:      "/usr/bin/llvm-ar-18",
			ArArgs:      []string{"--format=bsd"},
			Variant:     BSD,
			Prereq:      func(os string) bool { return os == "Linux" },
		},
		{
			Description: "macOS ar",
			ArPath:      "/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/bin/ar",
			ArArgs:      []string{},
			Variant:     BSD,
			Prereq:      func(os string) bool { return os == "macOS" },
		},
	} {
		t.Run(tc.Description, func(t *testing.T) {
			if !tc.Prereq(os.Getenv("RUNNER_OS")) {
				t.Skip("prerequisites not met")
			}

			tmp := filepath.Join(t.TempDir(), "test.a")
			f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE, 0644)
			require.NoError(t, err)

			writer := NewWriter(f, tc.Variant)
			if tc.Variant == GNU {
				err = writer.WriteStringTable(longFileNames)
			}
			for i, fileName := range fileNames {
				data := fmt.Sprintf("The name of this file contains %d character(s).\n", i+1)
				err := writer.WriteHeader(&Header{
					Name: fileName,
					Mode: 0600,
					Size: int64(len(data)),
				})
				require.NoError(t, err)
				n, err := writer.Write([]byte(data))
				assert.Equal(t, len(data), n)
				require.NoError(t, err)
			}
			err = writer.Close()
			require.NoError(t, err)
			err = f.Close()
			require.NoError(t, err)

			out, err := exec.Command(tc.ArPath, append(tc.ArArgs, "-x", tmp)...).CombinedOutput()
			if !assert.NoError(t, err) {
				t.Fatalf("%s output:\n%s\n", tc.ArPath, out)
			}

			for i, fileName := range fileNames {
				fi, err := os.Stat(fileName)
				require.NoError(t, err)
				assert.EqualValues(t, 0600, fi.Mode().Perm())
				f, err := os.ReadFile(fileName)
				require.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("The name of this file contains %d character(s).\n", i+1), string(f))
			}
		})
	}
}
