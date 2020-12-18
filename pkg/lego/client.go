package lego

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	rb "github.com/open-policy-agent/opa/bundle"
	"github.com/open-policy-agent/opa/loader"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/topdown"
	"github.com/rueian/opalego/pkg/bundle"
	"github.com/rueian/opalego/pkg/untar"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type BundleFetcher interface {
	Fetch() (bundle.Service, error)
}

type Lego struct {
	f        bundle.Factory
	c        Client
	r        *SidecarOPA
	debug    *DebugOption
	schedule sync.Once
}

func NewLego(f bundle.Factory, options ...func(*Lego)) *Lego {
	l := &Lego{f: f}
	for _, o := range options {
		o(l)
	}
	if l.r == nil {
		l.c = &LocalClient{l: l}
	} else {
		l.c = &RemoteClient{l: l}
	}
	return l
}

func WithSidecar(r SidecarOPA) func(*Lego) {
	return func(l *Lego) {
		l.r = &r
	}
}

func WithDebug(o DebugOption) func(*Lego) {
	return func(l *Lego) {
		l.debug = &o
	}
}

func (l *Lego) Client() Client {
	return l.c
}

func (l *Lego) ScheduleSetBundle(fetcher BundleFetcher, interval time.Duration, onDone func(error)) {
	l.schedule.Do(func() {
		go func() {
			for {
				data, err := fetcher.Fetch()
				if err == nil {
					err = l.SetBundle(data)
				}
				if onDone != nil {
					onDone(err)
				}
				time.Sleep(interval)
			}
		}()
	})
}

func (l *Lego) SetBundle(data bundle.Service) (err error) {
	var file *os.File
	if l.r != nil {
		file, err = os.Create(l.r.BundleDst + ".tmp")
	} else {
		file, err = ioutil.TempFile(os.TempDir(), "bundle-*.tar.gz")
	}
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())

	if err = l.f.WriteBundle(file, map[string]bundle.Service{"svc": data}); err != nil {
		file.Close()
		return err
	}

	if l.debug != nil && l.debug.UnTarBundleDir != "" {
		if _, err = file.Seek(0, 0); err == nil {
			err = untar.Untar(file, l.debug.UnTarBundleDir)
		}
	}
	file.Close()

	switch c := l.c.(type) {
	case *LocalClient:
		b, err := loader.NewFileLoader().AsBundle(file.Name())
		if err != nil {
			return err
		}
		c.setBundle(b)
	case *RemoteClient:
		return os.Rename(file.Name(), l.r.BundleDst)
	}
	return err
}

type SidecarOPA struct {
	Addr      string
	BundleDst string
}

type DebugOption struct {
	UnTarBundleDir string
	OnRequest      func(request interface{})
	OnResponse     func(response interface{})
}

type QueryOption struct {
	UID   string
	Rule  string
	Input map[string]interface{}
}

type Client interface {
	Query(ctx context.Context, option QueryOption) (output interface{}, err error)
}

type LocalClient struct {
	l      *Lego
	mu     sync.Mutex
	bundle *rb.Bundle
}

func (c *LocalClient) setBundle(bundle *rb.Bundle) {
	c.mu.Lock()
	c.bundle = bundle
	c.mu.Unlock()
}

func (c *LocalClient) Query(ctx context.Context, option QueryOption) (output interface{}, err error) {
	query, input := prepare(c.l.f.Mode, option)

	c.mu.Lock()
	options := []func(*rego.Rego){rego.Query(query), rego.Input(input), rego.ParsedBundle("svc", c.bundle)}
	c.mu.Unlock()

	var tracer *topdown.BufferTracer
	if c.l.debug != nil && c.l.debug.OnRequest != nil {
		tracer = topdown.NewBufferTracer()
		options = append(options, rego.QueryTracer(tracer))
		if c.l.debug.OnRequest != nil {
			c.l.debug.OnRequest(map[string]interface{}{"query": query, "input": input})
		}
	}

	rs, err := rego.New(options...).Eval(ctx)

	if c.l.debug != nil && c.l.debug.OnResponse != nil {
		c.l.debug.OnResponse(map[string]interface{}{"rs": rs, "err": err, "tracer": tracer})
	}

	if err != nil || len(rs) == 0 || len(rs[0].Bindings) == 0 {
		return nil, err
	}
	return rs[0].Bindings["x"], nil
}

type RemoteClient struct {
	l *Lego
}

func (c *RemoteClient) Query(ctx context.Context, option QueryOption) (output interface{}, err error) {
	query, input := prepare(c.l.f.Mode, option)
	bs, err := json.Marshal(map[string]interface{}{
		"query": query,
		"input": input,
	})
	if err != nil {
		return nil, err
	}

	endpoint := c.l.r.Addr + "/v1/query"
	if c.l.debug != nil {
		endpoint += "?explain=full"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	if c.l.debug != nil && c.l.debug.OnRequest != nil {
		c.l.debug.OnRequest(map[string]interface{}{"req": req, "body": string(bs)})
	}

	resp, err := http.DefaultClient.Do(req)

	if c.l.debug != nil && c.l.debug.OnResponse != nil {
		bs = nil
		defer func() {
			c.l.debug.OnResponse(map[string]interface{}{"resp": resp, "err": err, "body": string(bs)})
		}()
	}

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	bs, _ = ioutil.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("opa error: %d: %s", resp.StatusCode, bs)
	}

	result := map[string]interface{}{}
	if err = json.Unmarshal(bs, &result); err == nil {
		rs := result["result"]
		if rse, ok := rs.([]interface{}); ok && len(rse) != 0 {
			rs = rse[0]
		}
		if kv, ok := rs.(map[string]interface{}); ok {
			return kv["x"], nil
		}
	}
	return
}

func prepare(mode bundle.Mode, option QueryOption) (query string, input map[string]interface{}) {
	input = make(map[string]interface{}, len(option.Input))
	for k, v := range option.Input {
		input[k] = v
	}
	switch mode {
	case bundle.FlattenMode:
		query = fmt.Sprintf("x := data.svc.members.%s.%s", bundle.Normalize(option.UID), option.Rule)
		query = strings.TrimRight(query, ".")
	case bundle.GroupMode:
		query = fmt.Sprintf("y := data.svc.groups[data.svc.memberships.%s[_]].%s", bundle.Normalize(option.UID), option.Rule)
		query = strings.TrimRight(query, ".")
		query = fmt.Sprintf("x := [y | %s]", query)
	case bundle.DataMode:
		query = fmt.Sprintf("x := data.svc.%s", option.Rule)
		query = strings.TrimRight(query, ".")
		input["uid"] = bundle.Normalize(option.UID)
	}
	return
}
