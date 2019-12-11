package bindiff

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/dsnet/compress/bzip2"
	"io"
	"io/ioutil"
)

var patchError = errors.New("bindiff: corrupt patch")

func Patch(old io.Reader, new io.Writer, patch io.Reader) error {
	var header header
	err := binary.Read(patch, signMagnitudeLittleEndian{}, &header)
	if err != nil {
		return err
	}

	if header.Magic != magic {
		return patchError
	}

	if header.ControlLength < 0 || header.DiffLength < 0 || header.NewSize < 0 {
		return patchError
	}

	controlBuffer := make([]byte, header.ControlLength)
	_, err = io.ReadFull(patch, controlBuffer)
	if err != nil {
		return err
	}
	controlReader, err := bzip2.NewReader(bytes.NewReader(controlBuffer), &bzip2.ReaderConfig{})
	if err != nil {
		return err
	}

	diffBuffer := make([]byte, header.DiffLength)
	_, err = io.ReadFull(patch, diffBuffer)
	if err != nil {
		return err
	}
	diffReader, err := bzip2.NewReader(bytes.NewReader(diffBuffer), &bzip2.ReaderConfig{})
	if err != nil {
		return err
	}

	// The entire rest of the file is the extra block.
	extraReader, err := bzip2.NewReader(patch, &bzip2.ReaderConfig{})
	if err != nil {
		return err
	}

	oldBuffer, err := ioutil.ReadAll(old)
	if err != nil {
		return err
	}

	newBuffer := make([]byte, header.NewSize)

	var oldPosition, newPosition int64
	for newPosition < header.NewSize {
		var ctrl struct{ Add, Copy, Seek int64 }
		err = binary.Read(controlReader, signMagnitudeLittleEndian{}, &ctrl)
		if err != nil {
			return err
		}

		// Sanity-check
		if newPosition+ctrl.Add > header.NewSize {
			return patchError
		}

		// Read diff string
		_, err = io.ReadFull(diffReader, newBuffer[newPosition:newPosition+ctrl.Add])
		if err != nil {
			return patchError
		}

		// Add old data to diff string
		for i := int64(0); i < ctrl.Add; i++ {
			if oldPosition+i >= 0 && oldPosition+i < int64(len(oldBuffer)) {
				newBuffer[newPosition+i] += oldBuffer[oldPosition+i]
			}
		}

		// Adjust pointers
		newPosition += ctrl.Add
		oldPosition += ctrl.Add

		// Sanity-check
		if newPosition+ctrl.Copy > header.NewSize {
			return patchError
		}

		// Read extra string
		_, err = io.ReadFull(extraReader, newBuffer[newPosition:newPosition+ctrl.Copy])
		if err != nil {
			return patchError
		}

		// Adjust pointers
		newPosition += ctrl.Copy
		oldPosition += ctrl.Seek
	}

	// Write the new file
	for len(newBuffer) > 0 {
		n, err := new.Write(newBuffer)
		if err != nil {
			return err
		}
		newBuffer = newBuffer[n:]
	}

	return nil
}
