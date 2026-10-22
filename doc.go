// Package bindiff creates and applies binary patches in the bsdiff 4.x
// format described at https://www.daemonology.net/bsdiff/.
//
// Patches made by Diff can be applied by the reference bspatch, and Patch
// applies patches made by the reference bsdiff.
//
// A patch is a 32-byte header followed by three bzip2-compressed blocks:
//
//   - control: triples of (add, copy, seek) lengths
//   - diff: bytes added to the old data, one per byte of each add run
//   - extra: new bytes copied as-is, one per byte of each copy run
//
// The header holds the magic "BSDIFF40", the compressed lengths of the
// control and diff blocks, and the size of the new data, each as a 64-bit
// little-endian sign-magnitude integer.
//
// The bsdiff format carries no checksums, so a patch applied to the wrong
// input produces the wrong output without an error. Diff therefore appends
// a 72-byte trailer after the extra block: the SHA-256 of the old data, the
// SHA-256 of the new data, and the magic "BDSHA256". Patch checks both when
// the trailer is present. The reference bspatch reads only the bytes the
// control triples ask for, so it ignores the trailer.
package bindiff
