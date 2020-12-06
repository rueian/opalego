package lego

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Rego interface {
	Path() string
	Generate() []byte
}

type Member struct {
	f         *Factory
	pkg       string
	RegoBase  string
	RegoPiece []string
	Groups    []string
}

func (m *Member) Path() string {
	return path.Join(m.f.Package, "members", m.pkg)
}

func (m *Member) Generate() []byte {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "package %s\n%s\n", strings.ReplaceAll(m.Path(), "/", "."), m.RegoBase)

	pieces := map[string]bool{}
	for _, p := range m.RegoPiece {
		pieces[p] = true
	}
	for _, k := range m.Groups {
		if g, ok := m.f.Groups[k]; ok {
			for _, p := range g.RegoPiece {
				pieces[p] = true
			}
		}
	}

	for k := range pieces {
		if p, ok := m.f.RegoPiece[k]; ok {
			fmt.Fprintln(buf, p)
		}
	}

	return buf.Bytes()
}

type Group struct {
	f         *Factory
	pkg       string
	RegoBase  string
	RegoPiece []string
}

func (g *Group) Path() string {
	return path.Join(g.f.Package, "groups", g.pkg)
}

func (g *Group) Generate() []byte {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "package %s\n%s\n", strings.ReplaceAll(g.Path(), "/", "."), g.RegoBase)

	pieces := map[string]bool{}
	for _, p := range g.RegoPiece {
		pieces[p] = true
	}
	for k := range pieces {
		if p, ok := g.f.RegoPiece[k]; ok {
			fmt.Fprintln(buf, p)
		}
	}

	return buf.Bytes()
}

type Membership struct {
	f   *Factory
	pkg string
}

func (m *Membership) Path() string {
	return path.Join(m.f.Package, "membership", m.pkg)
}

func (m *Membership) Generate() []byte {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "package %s\n", strings.ReplaceAll(m.Path(), "/", "."))
	for k, member := range m.f.Members {
		groups := make([]string, 0, len(member.Groups))
		for _, v := range member.Groups {
			if _, ok := m.f.Groups[v]; ok {
				groups = append(groups, v)
			}
		}
		bs, _ := json.Marshal(groups)
		fmt.Fprintf(buf, "%s = %s\n", k, bs)
	}
	return buf.Bytes()
}

type BundleMode int

const (
	FlattenMode = iota
	GroupMode
	RawMode
)

type Factory struct {
	Package   string
	RegoBase  string
	RegoPiece map[string]string
	Members   map[string]Member
	Groups    map[string]Group

	Mode BundleMode
}

func (f *Factory) Path() string {
	return path.Join(f.Package, "main")
}

func (f *Factory) Generate() []byte {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "package %s\n%s\n", f.Package, f.RegoBase)
	return buf.Bytes()
}

func (f *Factory) WriteBundle(root string) error {
	// write base
	if err := writeRego(root, f); err != nil {
		return err
	}

	switch f.Mode {
	case FlattenMode:
		for k, p := range f.Members {
			p.f = f
			p.pkg = k
			if err := writeRego(root, &p); err != nil {
				return err
			}
		}
	case GroupMode:
		for k, p := range f.Groups {
			p.f = f
			p.pkg = k
			if err := writeRego(root, &p); err != nil {
				return err
			}
		}
		if err := writeRego(root, &Membership{f: f, pkg: ""}); err != nil {
			return err
		}
	case RawMode:
		// write service data, including roles
		// write members roles
	}

	bs, _ := json.Marshal(Manifest{Roots: []string{f.Package}})
	return ioutil.WriteFile(path.Join(root, ".manifest"), bs, os.ModePerm)
}

type Manifest struct {
	Roots []string `json:"roots"`
}

func writeRego(root string, f Rego) error {
	_ = os.MkdirAll(path.Join(root, filepath.Dir(f.Path())), os.ModePerm)
	return ioutil.WriteFile(path.Join(root, f.Path()+".rego"), f.Generate(), os.ModePerm)
}
