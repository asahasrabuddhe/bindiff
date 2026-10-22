package bindiff

import (
	"bytes"
	"crypto/sha256"
	"errors"
)

// ErrChecksumMismatch is returned by Patch when the old data, or the data
// it produced, does not match the checksums stored in the patch.
var ErrChecksumMismatch = errors.New("bindiff: checksum mismatch")

// trailerMagic ends the checksum trailer that Diff appends to every patch.
var trailerMagic = [8]byte{'B', 'D', 'S', 'H', 'A', '2', '5', '6'}

// trailerSize is the size of the trailer: the SHA-256 of the old data, the
// SHA-256 of the new data, and trailerMagic.
const trailerSize = 2*sha256.Size + len(trailerMagic)

// The bsdiff 4.x format has no checksums, so a patch applied to the wrong
// input silently produces the wrong output. Diff appends a trailer after
// the extra block to fix that. The reference bspatch reads only the bytes
// the control triples ask for, so it ignores the trailer.
type trailer struct {
	old, new [sha256.Size]byte
}

func (t trailer) append(b []byte) []byte {
	b = append(b, t.old[:]...)
	b = append(b, t.new[:]...)
	return append(b, trailerMagic[:]...)
}

// splitTrailer returns rest without its trailer, and the trailer if there
// is one. A bzip2 stream ending in the 8-byte magic by chance is too
// unlikely to matter.
func splitTrailer(rest []byte) ([]byte, *trailer) {
	if len(rest) < trailerSize || !bytes.Equal(rest[len(rest)-len(trailerMagic):], trailerMagic[:]) {
		return rest, nil
	}

	t := rest[len(rest)-trailerSize:]
	var tr trailer
	copy(tr.old[:], t[:sha256.Size])
	copy(tr.new[:], t[sha256.Size:2*sha256.Size])
	return rest[:len(rest)-trailerSize], &tr
}
