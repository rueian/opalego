package bundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

type Data map[string]Service

type Service struct {
	Base
	f       *Factory
	Members map[string]Member `json:"members"`
	Groups  map[string]Group  `json:"groups"`
}

type Group struct {
	Base
	Roles []string `json:"roles"`
}

type Member struct {
	Base
	Roles  []string `json:"roles"`
	Groups []string `json:"groups"`
}

type Manifest struct {
	Roots []string `json:"roots"`
}

type Base struct {
	Rego  string                 `json:"rego"`
	Extra map[string]interface{} `json:"extra"`
}

func (d Data) Generate(f *Factory) map[string]*bytes.Buffer {
	list := make(map[string]*bytes.Buffer)
	mani := Manifest{}
	for k, v := range d {
		v.f = f
		v.Generate(list, k)
		mani.Roots = append(mani.Roots, k)
	}
	list[".manifest"] = &bytes.Buffer{}
	json.NewEncoder(list[".manifest"]).Encode(mani)
	return list
}

func (s *Service) Generate(list map[string]*bytes.Buffer, root string) {
	s.Base.Generate(list, root, s)
	switch s.f.Mode {
	case FlattenMode:
		for k, m := range s.Members {
			m.Generate(list, path.Join(root, "members", k), s)
		}
	case GroupMode:
		for k, m := range s.Groups {
			m.Generate(list, path.Join(root, "groups", k), s)
		}
		memberships := map[string][]string{}
		for k, m := range s.Members {
			memberships[k] = m.Groups
		}
		list["memberships/data.json"] = &bytes.Buffer{}
		json.NewEncoder(list["memberships/data.json"]).Encode(memberships)
	case DataMode:
		memberroles := map[string][]string{}
		for k, m := range s.Members {
			memberroles[k] = fullRoles(&m, s)
		}
		list["memberroles/data.json"] = &bytes.Buffer{}
		json.NewEncoder(list["memberroles/data.json"]).Encode(memberroles)
	}
}

func (m *Member) Generate(list map[string]*bytes.Buffer, root string, s *Service) {
	m.Base.Generate(list, root, s)
	buf := list[path.Join(root, "main.rego")]
	for _, k := range fullRoles(m, s) {
		if p, ok := s.f.RegoPiece[k]; ok {
			fmt.Fprintln(buf, p)
		}
	}
}

func (m *Group) Generate(list map[string]*bytes.Buffer, root string, s *Service) {
	m.Base.Generate(list, root, s)
	buf := list[path.Join(root, "main.rego")]
	for _, k := range m.Roles {
		if p, ok := s.f.RegoPiece[k]; ok {
			fmt.Fprintln(buf, p)
		}
	}
}

func (b *Base) Generate(list map[string]*bytes.Buffer, root string, s *Service) {
	main := &bytes.Buffer{}
	fmt.Fprintf(main, "package %s\n%s\n", strings.ReplaceAll(root, "/", "."), b.Rego)
	list[path.Join(root, "main.rego")] = main

	if b.Extra != nil {
		data := &bytes.Buffer{}
		json.NewEncoder(data).Encode(b.Extra)
		list[path.Join(root, "extra", "data.json")] = data
	}
}

func fullRoles(m *Member, s *Service) []string {
	roles := map[string]bool{}
	for _, p := range m.Roles {
		roles[p] = true
	}
	for _, k := range m.Groups {
		if g, ok := s.Groups[k]; ok {
			for _, p := range g.Roles {
				roles[p] = true
			}
		}
	}
	list := make([]string, 0, len(roles))
	for k := range roles {
		list = append(list, k)
	}
	return list
}
