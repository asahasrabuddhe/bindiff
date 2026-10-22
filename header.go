package bindiff

// magic identifies a bsdiff 4.x patch.
var magic = [8]byte{'B', 'S', 'D', 'I', 'F', 'F', '4', '0'}

// headerSize is the encoded size of header in bytes.
const headerSize = 32

// header is the fixed-size start of every patch. Lengths are the sizes of
// the compressed control and diff blocks that follow it.
type header struct {
	Magic         [8]byte
	ControlLength int64
	DiffLength    int64
	NewSize       int64
}
