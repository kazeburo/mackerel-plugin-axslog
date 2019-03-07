package posreader

import "io"

// Reader struct
type Reader struct {
	Pos    int64
	reader io.Reader
}

// New :
func New(ir io.Reader, pos int64) (*Reader, error) {
	if is, ok := ir.(io.Seeker); ok {
		_, err := is.Seek(pos, 0)
		if err != nil {
			return nil, err
		}
	}
	return &Reader{
		Pos:    pos,
		reader: ir,
	}, nil
}

// Read :
func (r *Reader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.Pos += int64(n)
	return n, err
}
