// Package lines reads the files placket is handed rather than writes itself: a journal being
// verified, logged or reconciled, and a state directory's files. Such a file is read only when it is
// a regular file, so a FIFO or a device never holds a command, and each of its lines is read with a
// cap. The file as a whole has none, since a journal grows for as long as its Accord runs.
package lines

import (
	"bufio"
	"errors"
	"io"
	"os"
	"syscall"
)

// ErrNotRegular is a path that names something other than a regular file.
var ErrNotRegular = errors.New("lines: not a regular file")

// NotRegular returns err, a package's own error for the file at a path that is not a regular file,
// so that errors.As still finds err in it and errors.Is finds ErrNotRegular too. Its text is err's.
func NotRegular(err error) error { return notRegular{err} }

type notRegular struct{ error }

func (n notRegular) Unwrap() []error { return []error{n.error, ErrNotRegular} }

// A File is what Read found in a file: its whole lines, without their line feeds, in order.
type File struct {
	Lines [][]byte
	// Cut is true when the file ends in a line with no line feed, which is left out of Lines.
	Cut bool
	// Long is the number, from 1, of the first line longer than the cap; the lines before it are
	// read and none after. It is 0 when no line is.
	Long int
}

// Read reads the regular file at path, each line at most max bytes before its line feed. A path
// that does not exist gives an error that matches fs.ErrNotExist, and one that is not a regular file,
// after following links, ErrNotRegular. Read checks the path before it opens it, and checks again
// that what it opened is the file it checked.
func Read(path string, max int) (File, error) {
	f, err := Open(path)
	if err != nil {
		return File{}, err
	}
	defer func() { _ = f.Close() }()
	var out File
	br := bufio.NewReader(f)
	for {
		l, cut, long, err := line(br, max)
		switch {
		case err != nil:
			return File{}, err
		case long:
			out.Long = len(out.Lines) + 1
			return out, nil
		case l == nil:
			return out, nil
		case cut:
			out.Cut = true
			return out, nil
		}
		out.Lines = append(out.Lines, l)
	}
}

// ReadAll reads the whole regular file at path, as Read checks it, when it holds at most max bytes;
// a longer one gives ok false.
func ReadAll(path string, max int) (b []byte, ok bool, err error) {
	f, err := Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()
	b, err = io.ReadAll(io.LimitReader(f, int64(max)+1))
	if err != nil {
		return nil, false, err
	}
	return b, len(b) <= max, nil
}

// Open opens the regular file at path for reading, following links: ErrNotRegular for anything
// else, checked before it is opened and again on what was opened.
func Open(path string) (*os.File, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	// O_NONBLOCK: should a FIFO take the path's place after the check, the open does not wait for a
	// writer, and the check of the opened file below refuses it.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !os.SameFile(fi, opened) {
		_ = f.Close()
		return nil, ErrNotRegular
	}
	return f, nil
}

// line reads one line of br: the line without its line feed, cut when it ended without one, long
// when it ran past max bytes. At the end of the input it returns a nil line.
func line(br *bufio.Reader, max int) (l []byte, cut, long bool, err error) {
	var b []byte
	for {
		chunk, err := br.ReadSlice('\n')
		b = append(b, chunk...)
		switch {
		case len(b) > max+1 || (len(b) > max && (err != nil || b[len(b)-1] != '\n')):
			return nil, false, true, nil
		case err == nil:
			return b[: len(b)-1 : len(b)-1], false, false, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(b) == 0 {
				return nil, false, false, nil
			}
			return b, true, false, nil
		default:
			return nil, false, false, err
		}
	}
}
