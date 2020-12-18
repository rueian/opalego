package bundle

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"regexp"
	"strings"
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

	for _, s := range data {
		for k, m := range s.Members {
			for i, g := range m.Groups {
				m.Groups[i] = Normalize(g)
			}
			if n := Normalize(k); n != k {
				s.Members[n] = m
				delete(s.Members, k)
			}
		}
		for k, g := range s.Groups {
			if n := Normalize(k); n != k {
				s.Groups[n] = g
				delete(s.Groups, k)
			}
		}
	}

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

func Normalize(s string) string {
	s = strings.ToLower(s)
	return "u" + normalizer.ReplaceAllString(s, "_")
}

var normalizer = regexp.MustCompile("[^a-z0-9]+")