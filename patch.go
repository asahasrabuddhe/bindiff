package bindiff

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/dsnet/compress/bzip2"
)

// ErrCorruptPatch is returned by Patch when the patch is malformed or does
// not fit the old data.
var ErrCorruptPatch = errors.New("bindiff: corrupt patch")

// PatchError is the v0.1.x name of ErrCorruptPatch.
//
// Deprecated: Use ErrCorruptPatch.
var PatchError = ErrCorruptPatch

// chunkSize bounds how much of the new file Patch holds in memory at once.
const chunkSize = 32 << 10

func corrupt(what string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrCorruptPatch, what)
	}
	return fmt.Errorf("%w: %s: %w", ErrCorruptPatch, what, err)
}

// Patch reads a bsdiff 4.x patch from patch, applies it to the contents of
// old and writes the result to new.
//
// If the patch carries the checksum trailer that Diff writes, Patch checks
// old against it before writing anything, and checks the result at the
// end. Either mismatch returns an error wrapping ErrChecksumMismatch.
// Patches without the trailer, such as those from the reference bsdiff,
// are applied without the checks.
//
// Patch reads all of old into memory and streams the result to new. If the
// patch turns out to be corrupt part way through, new may already hold a
// partial result, so write to a temporary file and rename it once Patch
// returns nil.
func Patch(old io.Reader, new io.Writer, patch io.Reader) error { //nolint:revive // old and new read best here.
	var h header
	if err := binary.Read(patch, signMagnitudeLittleEndian{}, &h); err != nil {
		return corrupt("read header", err)
	}

	if h.Magic != magic {
		return corrupt(fmt.Sprintf("bad magic %q", h.Magic[:]), nil)
	}

	if h.ControlLength < 0 || h.DiffLength < 0 || h.NewSize < 0 {
		return corrupt("negative length in header", nil)
	}

	controlBlock, err := readBlock(patch, h.ControlLength)
	if err != nil {
		return corrupt("read control block", err)
	}

	diffBlock, err := readBlock(patch, h.DiffLength)
	if err != nil {
		return corrupt("read diff block", err)
	}

	controlReader, err := bzip2.NewReader(bytes.NewReader(controlBlock), nil)
	if err != nil {
		return corrupt("open control block", err)
	}

	diffReader, err := bzip2.NewReader(bytes.NewReader(diffBlock), nil)
	if err != nil {
		return corrupt("open diff block", err)
	}

	// The rest of the patch is the extra block and, from Diff, a checksum
	// trailer.
	rest, err := io.ReadAll(patch)
	if err != nil {
		return corrupt("read extra block", err)
	}
	extraBlock, sums := splitTrailer(rest)

	extraReader, err := bzip2.NewReader(bytes.NewReader(extraBlock), nil)
	if err != nil {
		return corrupt("open extra block", err)
	}

	oldBuffer, err := io.ReadAll(old)
	if err != nil {
		return err
	}

	if sums != nil && sha256.Sum256(oldBuffer) != sums.old {
		return fmt.Errorf("%w: old data is not the data this patch was made from", ErrChecksumMismatch)
	}
	newHash := sha256.New()

	// Small outputs need small buffers. bufio rounds sizes under 16 up.
	bufSize := int(min(h.NewSize, chunkSize))
	w := bufio.NewWriterSize(new, bufSize)
	chunk := make([]byte, bufSize)

	var oldPosition, newPosition int64
	for newPosition < h.NewSize {
		var ctrl struct{ Add, Copy, Seek int64 }
		if err := binary.Read(controlReader, signMagnitudeLittleEndian{}, &ctrl); err != nil {
			return corrupt("read control triple", err)
		}

		if ctrl.Add < 0 || ctrl.Copy < 0 {
			return corrupt("negative length in control triple", nil)
		}

		if ctrl.Add > h.NewSize-newPosition {
			return corrupt("diff runs past end of new file", nil)
		}

		// Add old data to the diff string, one chunk at a time.
		for remaining := ctrl.Add; remaining > 0; {
			n := min(remaining, int64(len(chunk)))
			buf := chunk[:n]
			if _, err := io.ReadFull(diffReader, buf); err != nil {
				return corrupt("read diff block", err)
			}

			for i := range buf {
				if p := oldPosition + int64(i); p >= 0 && p < int64(len(oldBuffer)) {
					buf[i] += oldBuffer[p]
				}
			}

			newHash.Write(buf)
			if _, err := w.Write(buf); err != nil {
				return err
			}

			oldPosition += n
			remaining -= n
		}
		newPosition += ctrl.Add

		if ctrl.Copy > h.NewSize-newPosition {
			return corrupt("extra runs past end of new file", nil)
		}

		for remaining := ctrl.Copy; remaining > 0; {
			buf := chunk[:min(remaining, int64(len(chunk)))]
			if _, err := io.ReadFull(extraReader, buf); err != nil {
				return corrupt("read extra block", err)
			}

			newHash.Write(buf)
			if _, err := w.Write(buf); err != nil {
				return err
			}

			remaining -= int64(len(buf))
		}
		newPosition += ctrl.Copy

		oldPosition += ctrl.Seek
	}

	// Reading each block to its end makes the bzip2 reader check the stream
	// checksum, and catches trailing data the control triples never used.
	for _, b := range []struct {
		name string
		r    io.Reader
	}{
		{"control block", controlReader},
		{"diff block", diffReader},
		{"extra block", extraReader},
	} {
		if err := expectEOF(b.r); err != nil {
			return corrupt(b.name, err)
		}
	}

	if err := w.Flush(); err != nil {
		return err
	}

	if sums != nil && [sha256.Size]byte(newHash.Sum(nil)) != sums.new {
		return fmt.Errorf("%w: patched data does not match the patch", ErrChecksumMismatch)
	}
	return nil
}

// readBlock reads exactly n bytes from r. It grows its buffer as data
// arrives, so a huge n in a corrupt header cannot force a huge allocation.
func readBlock(r io.Reader, n int64) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := io.CopyN(&buf, r, n); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}

	return buf.Bytes(), nil
}

// expectEOF reports an error unless r has no data left.
func expectEOF(r io.Reader) error {
	var b [1]byte
	n, err := io.ReadFull(r, b[:])
	switch {
	case n > 0:
		return errors.New("unexpected trailing data")
	case errors.Is(err, io.EOF):
		return nil
	default:
		return err
	}
}
