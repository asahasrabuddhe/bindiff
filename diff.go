package bindiff

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"

	"github.com/dsnet/compress/bzip2"
)

// Diff reads old and new in full, computes a bsdiff 4.x patch that turns old
// into new, and writes it to patch. The patch ends with the SHA-256 of old
// and new, which Patch checks and the reference bspatch ignores.
//
// Diff holds both inputs and the patch in memory, plus a suffix array of
// old that needs about 16 bytes per byte of old.
func Diff(old, new io.Reader, patch io.Writer) error { //nolint:revive // old and new read best here.
	oldBuffer, err := io.ReadAll(old)
	if err != nil {
		return err
	}

	newBuffer, err := io.ReadAll(new)
	if err != nil {
		return err
	}

	return diff(oldBuffer, newBuffer, patch)
}

func swap(a []int, i, j int) {
	a[i], a[j] = a[j], a[i]
}

func split(I, V []int, start, length, h int) {
	var i, j, k, x, jj, kk int

	if length < 16 {
		for k = start; k < start+length; k += j {
			j = 1
			x = V[I[k]+h]
			for i = 1; k+i < start+length; i++ {
				if V[I[k+i]+h] < x {
					x = V[I[k+i]+h]
					j = 0
				}
				if V[I[k+i]+h] == x {
					swap(I, k+i, k+j)
					j++
				}
			}
			for i = 0; i < j; i++ {
				V[I[k+i]] = k + j - 1
			}
			if j == 1 {
				I[k] = -1
			}
		}
		return
	}

	x = V[I[start+length/2]+h]
	jj = 0
	kk = 0
	for i = start; i < start+length; i++ {
		if V[I[i]+h] < x {
			jj++
		}
		if V[I[i]+h] == x {
			kk++
		}
	}
	jj += start
	kk += jj

	i = start
	j = 0
	k = 0
	for i < jj {
		switch v := V[I[i]+h]; {
		case v < x:
			i++
		case v == x:
			swap(I, i, jj+j)
			j++
		default:
			swap(I, i, kk+k)
			k++
		}
	}

	for jj+j < kk {
		if V[I[jj+j]+h] == x {
			j++
		} else {
			swap(I, jj+j, kk+k)
			k++
		}
	}

	if jj > start {
		split(I, V, start, jj-start, h)
	}

	for i = 0; i < kk-jj; i++ {
		V[I[jj+i]] = kk - 1
	}
	if jj == kk-1 {
		I[jj] = -1
	}

	if start+length > kk {
		split(I, V, kk, start+length-kk, h)
	}
}

// quickSuffixSort returns the suffix array of buffer, using the
// Larsson-Sadakane qsufsort algorithm from bsdiff.
func quickSuffixSort(buffer []byte) []int {
	var buckets [256]int
	var i, h int
	I := make([]int, len(buffer)+1)
	V := make([]int, len(buffer)+1)

	for _, c := range buffer {
		buckets[c]++
	}
	for i = 1; i < 256; i++ {
		buckets[i] += buckets[i-1]
	}
	copy(buckets[1:], buckets[:])
	buckets[0] = 0

	for i, c := range buffer {
		buckets[c]++
		I[buckets[c]] = i
	}

	I[0] = len(buffer)
	for i, c := range buffer {
		V[i] = buckets[c]
	}

	V[len(buffer)] = 0
	for i = 1; i < 256; i++ {
		if buckets[i] == buckets[i-1]+1 {
			I[buckets[i]] = -1
		}
	}
	I[0] = -1

	for h = 1; I[0] != -(len(buffer) + 1); h += h {
		var n int
		for i = 0; i < len(buffer)+1; {
			if I[i] < 0 {
				n -= I[i]
				i -= I[i]
			} else {
				if n != 0 {
					I[i-n] = -n
				}
				n = V[I[i]] + 1 - i
				split(I, V, i, n, h)
				i += n
				n = 0
			}
		}
		if n != 0 {
			I[i-n] = -n
		}
	}

	for i = 0; i < len(buffer)+1; i++ {
		I[V[i]] = i
	}
	return I
}

func matchLength(a, b []byte) (i int) {
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	return i
}

// search finds the longest prefix of newBuffer that occurs in oldBuffer,
// by binary search over the suffix array I between start and end.
func search(I []int, oldBuffer, newBuffer []byte, start, end int) (pos, n int) {
	for end-start >= 2 {
		x := start + (end-start)/2
		if bytes.Compare(oldBuffer[I[x]:], newBuffer) < 0 {
			start = x
		} else {
			end = x
		}
	}

	x := matchLength(oldBuffer[I[start]:], newBuffer)
	y := matchLength(oldBuffer[I[end]:], newBuffer)
	if x > y {
		return I[start], x
	}
	return I[end], y
}

func diff(oldBuffer, newBuffer []byte, patch io.Writer) error {
	var lenf int
	I := quickSuffixSort(oldBuffer)
	diffBytes := make([]byte, len(newBuffer))
	extraBytes := make([]byte, len(newBuffer))
	var diffBytesLength, extraBytesLength int

	// Control triples are 24 bytes each, so build the block in memory.
	var ctrl []byte

	var scan, pos, length int
	var lastScan, lastPosition, lastOffset int
	for scan < len(newBuffer) {
		var oldScore int
		scan += length
		for scn := scan; scan < len(newBuffer); scan++ {
			pos, length = search(I, oldBuffer, newBuffer[scan:], 0, len(oldBuffer))

			for ; scn < scan+length; scn++ {
				if scn+lastOffset < len(oldBuffer) &&
					oldBuffer[scn+lastOffset] == newBuffer[scn] {
					oldScore++
				}
			}

			if (length == oldScore && length != 0) || length > oldScore+8 {
				break
			}

			if scan+lastOffset < len(oldBuffer) && oldBuffer[scan+lastOffset] == newBuffer[scan] {
				oldScore--
			}
		}

		if length != oldScore || scan == len(newBuffer) {
			var s, Sf int
			lenf = 0
			for i := 0; lastScan+i < scan && lastPosition+i < len(oldBuffer); {
				if oldBuffer[lastPosition+i] == newBuffer[lastScan+i] {
					s++
				}
				i++
				if s*2-i > Sf*2-lenf {
					Sf = s
					lenf = i
				}
			}

			bufferLength := 0
			if scan < len(newBuffer) {
				var s, Sb int
				for i := 1; (scan >= lastScan+i) && (pos >= i); i++ {
					if oldBuffer[pos-i] == newBuffer[scan-i] {
						s++
					}
					if s*2-i > Sb*2-bufferLength {
						Sb = s
						bufferLength = i
					}
				}
			}

			if lastScan+lenf > scan-bufferLength {
				overlap := (lastScan + lenf) - (scan - bufferLength)
				s := 0
				Ss := 0
				lens := 0
				for i := range overlap {
					if newBuffer[lastScan+lenf-overlap+i] == oldBuffer[lastPosition+lenf-overlap+i] {
						s++
					}
					if newBuffer[scan-bufferLength+i] == oldBuffer[pos-bufferLength+i] {
						s--
					}
					if s > Ss {
						Ss = s
						lens = i + 1
					}
				}

				lenf += lens - overlap
				bufferLength -= lens
			}

			for i := 0; i < lenf; i++ {
				diffBytes[diffBytesLength+i] = newBuffer[lastScan+i] - oldBuffer[lastPosition+i]
			}
			for i := 0; i < (scan-bufferLength)-(lastScan+lenf); i++ {
				extraBytes[extraBytesLength+i] = newBuffer[lastScan+lenf+i]
			}

			diffBytesLength += lenf
			extraBytesLength += (scan - bufferLength) - (lastScan + lenf)

			ctrl = appendInt64(ctrl, int64(lenf))
			ctrl = appendInt64(ctrl, int64((scan-bufferLength)-(lastScan+lenf)))
			ctrl = appendInt64(ctrl, int64((pos-bufferLength)-(lastPosition+lenf)))

			lastScan = scan - bufferLength
			lastPosition = pos - bufferLength
			lastOffset = pos - scan
		}
	}
	controlBlock, err := compress(ctrl)
	if err != nil {
		return err
	}

	diffBlock, err := compress(diffBytes[:diffBytesLength])
	if err != nil {
		return err
	}

	extraBlock, err := compress(extraBytes[:extraBytesLength])
	if err != nil {
		return err
	}

	h := header{
		Magic:         magic,
		ControlLength: int64(len(controlBlock)),
		DiffLength:    int64(len(diffBlock)),
		NewSize:       int64(len(newBuffer)),
	}
	if err := binary.Write(patch, signMagnitudeLittleEndian{}, &h); err != nil {
		return err
	}

	sums := trailer{old: sha256.Sum256(oldBuffer), new: sha256.Sum256(newBuffer)}
	for _, block := range [][]byte{controlBlock, diffBlock, extraBlock, sums.append(nil)} {
		if _, err := patch.Write(block); err != nil {
			return err
		}
	}

	return nil
}

// compress returns p compressed with bzip2 at the highest level, as bsdiff
// does.
func compress(p []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := bzip2.NewWriter(&buf, &bzip2.WriterConfig{Level: bzip2.BestCompression})
	if err != nil {
		return nil, err
	}

	if _, err := w.Write(p); err != nil {
		return nil, err
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// appendInt64 appends v to b in the sign-magnitude encoding bsdiff uses.
func appendInt64(b []byte, v int64) []byte {
	var buf [8]byte
	signMagnitudeLittleEndian{}.PutUint64(buf[:], uint64(v))
	return append(b, buf[:]...)
}
