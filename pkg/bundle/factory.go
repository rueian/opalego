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

	for k, s := range data {
		nm := make(map[string]Member, len(s.Members))
		ng := make(map[string]Group, len(s.Groups))
		for k, m := range s.Members {
			m.Groups = NormalizeSlice(m.Groups)
			nm[Normalize(k)] = m
		}
		for k, g := range s.Groups {
			ng[Normalize(k)] = g
		}
		s.Members = nm
		s.Groups = ng
		data[k] = s
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
	return "g" + normalizer.ReplaceAllString(s, "_")
}

func NormalizeSlice(s []string) []string {
	o := make([]string, len(s))
	for i, v := range s {
		o[i] = Normalize(v)
	}
	return o
}

var normalizer = regexp.MustCompile("[^a-z0-9]+")