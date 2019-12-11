package bindiff

import (
	"io"
	"io/ioutil"
	"math/rand"
	"os"
)

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

func mustOpen(path string) *os.File {
	file, err := os.Open(path)
	panicOnError(err)

	return file
}

func mustReadAll(r io.Reader) []byte {
	bytes, err := ioutil.ReadAll(r)
	panicOnError(err)

	return bytes
}

func mustRandBytes(n int) []byte {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	return b
}

func mustWriteRandomFile(pattern string, size int, seed int64) *os.File {
	p := make([]byte, size)
	rand.Seed(seed)

	_, err := rand.Read(p)
	panicOnError(err)

	file, err := ioutil.TempFile("/tmp", pattern)
	panicOnError(err)

	_, err = file.Write(p)
	panicOnError(err)

	_, err = file.Seek(0, 0)
	panicOnError(err)

	return file
}
