package bindiff_test

import (
	"bytes"
	"fmt"
	"log"

	"go.ajitem.com/bindiff"
)

func Example() {
	old := []byte("the quick brown fox jumps over the lazy dog")
	updated := []byte("the quick red fox jumps over the lazy cat")

	var patch bytes.Buffer
	if err := bindiff.Diff(bytes.NewReader(old), bytes.NewReader(updated), &patch); err != nil {
		log.Fatal(err)
	}

	var out bytes.Buffer
	if err := bindiff.Patch(bytes.NewReader(old), &out, &patch); err != nil {
		log.Fatal(err)
	}

	fmt.Println(out.String())
	// Output: the quick red fox jumps over the lazy cat
}
