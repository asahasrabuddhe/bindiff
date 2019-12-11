package bindiff

import (
	"bytes"
	"encoding/binary"
	"github.com/dsnet/compress/bzip2"
	"io"
	"io/ioutil"
)

func Diff(old, new io.Reader, patch io.Writer) error {
	oldBuffer, err := ioutil.ReadAll(old)
	if err != nil {
		return err
	}

	newBuffer, err := ioutil.ReadAll(new)
	if err != nil {
		return err
	}

	patchBuffer, err := diffBytes(oldBuffer, newBuffer)
	if err != nil {
		return err
	}

	_, err = patch.Write(patchBuffer)

	return err
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
		if V[I[i]+h] < x {
			i++
		} else if V[I[i]+h] == x {
			swap(I, i, jj+j)
			j++
		} else {
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

func search(I []int, oldBuffer, newBuffer []byte, start, end int) (pos, n int) {
	if end-start < 2 {
		x := matchLength(oldBuffer[I[start]:], newBuffer)
		y := matchLength(oldBuffer[I[end]:], newBuffer)

		if x > y {
			return I[start], x
		} else {
			return I[end], y
		}
	}

	x := start + (end-start)/2
	if bytes.Compare(oldBuffer[I[x]:], newBuffer) < 0 {
		return search(I, oldBuffer, newBuffer, x, end)
	} else {
		return search(I, oldBuffer, newBuffer, start, x)
	}
}

func diffBytes(oldBuffer, newBuffer []byte) ([]byte, error) {
	var patch seekBuffer
	err := diff(oldBuffer, newBuffer, &patch)
	if err != nil {
		return nil, err
	}
	return patch.buffer, nil
}

func diff(oldBuffer, newBuffer []byte, patch io.WriteSeeker) error {
	var lenf int
	I := quickSuffixSort(oldBuffer)
	diffBytes := make([]byte, len(newBuffer))
	extraBytes := make([]byte, len(newBuffer))
	var diffBytesLength, extraBytesLength int

	var header header
	header.Magic = magic
	header.NewSize = int64(len(newBuffer))
	err := binary.Write(patch, signMagnitudeLittleEndian{}, &header)
	if err != nil {
		return err
	}

	// Compute the differences, writing ctrl as we go
	writer, err := bzip2.NewWriter(patch, &bzip2.WriterConfig{Level: bzip2.BestCompression})
	if err != nil {
		return err
	}
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
				for i := 0; i < overlap; i++ {
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

			err = binary.Write(writer, signMagnitudeLittleEndian{}, int64(lenf))
			if err != nil {
				writer.Close()
				return err
			}

			val := (scan - bufferLength) - (lastScan + lenf)
			err = binary.Write(writer, signMagnitudeLittleEndian{}, int64(val))
			if err != nil {
				writer.Close()
				return err
			}

			val = (pos - bufferLength) - (lastPosition + lenf)
			err = binary.Write(writer, signMagnitudeLittleEndian{}, int64(val))
			if err != nil {
				writer.Close()
				return err
			}

			lastScan = scan - bufferLength
			lastPosition = pos - bufferLength
			lastOffset = pos - scan
		}
	}
	err = writer.Close()
	if err != nil {
		return err
	}

	// Compute size of compressed ctrl data
	l64, err := patch.Seek(0, 1)
	if err != nil {
		return err
	}
	header.ControlLength = l64 - 32

	// Write compressed diff data
	writer, err = bzip2.NewWriter(patch, &bzip2.WriterConfig{Level: bzip2.BestCompression})
	if err != nil {
		return err
	}
	n, err := writer.Write(diffBytes[:diffBytesLength])
	if err != nil {
		writer.Close()
		return err
	}
	if n != diffBytesLength {
		writer.Close()
		return io.ErrShortWrite
	}
	err = writer.Close()
	if err != nil {
		return err
	}

	// Compute size of compressed diff data
	n64, err := patch.Seek(0, 1)
	if err != nil {
		return err
	}
	header.DiffLength = n64 - l64

	// Write compressed extra data
	writer, err = bzip2.NewWriter(patch, &bzip2.WriterConfig{Level: bzip2.BestCompression})
	if err != nil {
		return err
	}
	n, err = writer.Write(extraBytes[:extraBytesLength])
	if err != nil {
		writer.Close()
		return err
	}
	if n != extraBytesLength {
		writer.Close()
		return io.ErrShortWrite
	}
	err = writer.Close()
	if err != nil {
		return err
	}

	// Seek to the beginning, write the header, and close the file
	_, err = patch.Seek(0, 0)
	if err != nil {
		return err
	}
	err = binary.Write(patch, signMagnitudeLittleEndian{}, &header)
	if err != nil {
		return err
	}
	return nil
}
