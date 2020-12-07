package bundle

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
)

type Mode int

const (
	FlattenMode = iota
	GroupMode
	DataMode
)

type Factory struct {
	RegoPiece map[string]string
	Mode      Mode
}

func (f *Factory) WriteBundle(root string, data Data) error {
	for p, buf := range data.Generate(f) {
		_ = os.MkdirAll(path.Join(root, filepath.Dir(p)), os.ModePerm)
		if err := ioutil.WriteFile(path.Join(root, p), buf.Bytes(), os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}
