package bindiff

var magic = [8]byte{'B', 'S', 'D', 'I', 'F', 'F', '4', '0'}

type header struct {
	Magic         [8]byte
	ControlLength int64
	DiffLength    int64
	NewSize       int64
}
