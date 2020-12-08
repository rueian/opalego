package lego

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	rb "github.com/open-policy-agent/opa/bundle"
	"github.com/open-policy-agent/opa/loader"
	"github.com/open-policy-agent/opa/rego"
	"github.com/rueian/opalego/pkg/bundle"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"
)

type BundleFetcher interface {
	Fetch() (bundle.Service, error)
}

type Lego struct {
	f        bundle.Factory
	r        SidecarOPA
	c        Client
	schedule sync.Once
}

func NewLego(f bundle.Factory, options ...func(*Lego)) *Lego {
	l := &Lego{f: f}
	for _, o := range options {
		o(l)
	}
	if l.r.Addr == "" {
		l.c = &LocalClient{f: f}
	} else {
		l.c = &RemoteClient{f: f, r: l.r}
	}
	return l
}

func WithSidecar(r SidecarOPA) func(*Lego) {
	return func(l *Lego) {
		l.r = r
	}
}

func (l *Lego) Client() Client {
	return l.c
}

func (l *Lego) ScheduleSetBundle(fetcher BundleFetcher, interval time.Duration, onErr func(error)) {
	l.schedule.Do(func() {
		go func() {
			for {
				data, err := fetcher.Fetch()
				if err == nil {
					err = l.SetBundle(data)
				}
				if err != nil && onErr != nil {
					onErr(err)
				}
				time.Sleep(interval)
			}
		}()
	})
}

func (l *Lego) SetBundle(data bundle.Service) error {
	file, err := ioutil.TempFile(os.TempDir(), "bundle-*.tar.gz")
	if err != nil {
		return nil
	}
	defer os.Remove(file.Name())

	if err := l.f.WriteBundle(file, map[string]bundle.Service{
		"svc": data,
	}); err != nil {
		file.Close()
		return err
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
	return nil
}

type SidecarOPA struct {
	Addr      string
	BundleDst string
}

type QueryOption struct {
	UID   string
	Rule  string
	Input map[string]interface{}
}

type Client interface {
	Query(ctx context.Context, option QueryOption) (output rego.ResultSet, err error)
}

type LocalClient struct {
	f      bundle.Factory
	mu     sync.Mutex
	bundle *rb.Bundle
}

func (l *LocalClient) setBundle(bundle *rb.Bundle) {
	l.mu.Lock()
	l.bundle = bundle
	l.mu.Unlock()
}

func (l *LocalClient) Query(ctx context.Context, option QueryOption) (output rego.ResultSet, err error) {
	query, input := prepare(l.f.Mode, option)
	l.mu.Lock()
	r := rego.New(
		rego.Query(query),
		rego.Input(input),
		rego.ParsedBundle("svc", l.bundle),
	)
	l.mu.Unlock()
	return r.Eval(ctx)
}

type RemoteClient struct {
	f bundle.Factory
	r SidecarOPA
}

func (r *RemoteClient) Query(ctx context.Context, option QueryOption) (output rego.ResultSet, err error) {
	query, input := prepare(r.f.Mode, option)
	bs, err := json.Marshal(map[string]interface{}{
		"query": query,
		"input": input,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.r.Addr+"/v1/query", bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("opa error: %d", resp.StatusCode)
	}
	err = json.NewDecoder(resp.Body).Decode(&output)
	return
}

func prepare(mode bundle.Mode, option QueryOption) (query string, input map[string]interface{}) {
	input = make(map[string]interface{}, len(option.Input))
	for k, v := range option.Input {
		input[k] = v
	}
	switch mode {
	case bundle.FlattenMode:
		query = fmt.Sprintf("data.svc.members.%s.%s", option.UID, option.Rule)
	case bundle.GroupMode:
		query = fmt.Sprintf("data.svc.groups.%s.%s", option.UID, option.Rule)
	case bundle.DataMode:
		query = fmt.Sprintf("data.svc.%s", option.Rule)
		input["uid"] = option.UID
	}
	return
}
