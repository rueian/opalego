package lego

import (
	"context"
	"github.com/open-policy-agent/opa/rego"
)

type QueryOption struct {
	Query string      `json:"query"`
	Input interface{} `json:"input"`
}

type PartialOption struct {
	Query    string      `json:"query"`
	Input    interface{} `json:"input"`
	Unknowns []string    `json:"unknowns"`
}

type Client interface {
	Eval(ctx context.Context, option QueryOption) (output rego.ResultSet, err error)
	Partial(ctx context.Context, option PartialOption) (output *rego.PartialQueries, err error)
}
