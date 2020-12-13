package bundle

import (
	"archive/tar"
	"compress/gzip"
	"io"
)

type Mode int

const (
	FlattenMode = iota
	GroupMode
	DataMode
)

type Factory struct {
	Base
	RegoPiece map[string]string `json:"regoPiece"`
	Mode      Mode              `json:"mode"`
}

func (f *Factory) WriteBundle(out io.Writer, data Data) error {
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for p, buf := range data.Generate(f) {
		if err := tw.WriteHeader(&tar.Header{Name: p, Mode: 0600, Size: int64(buf.Len())}); err != nil {
			return err
		}
		if _, err := tw.Write(buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}
