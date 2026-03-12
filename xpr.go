package xpr

import (
	"fmt"
	"time"
)

type Xpr struct {
	fns map[string]xprFunc
}

func New() *Xpr {
	return &Xpr{fns: map[string]xprFunc{}}
}

func normalizeCtxValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	case uint64:
		return float64(val)
	case uint32:
		return float64(val)
	case map[string]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, vv := range val {
			m[k] = normalizeCtxValue(vv)
		}
		return m
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(val))
		for k, vv := range val {
			m[fmt.Sprintf("%v", k)] = normalizeCtxValue(vv)
		}
		return m
	case []interface{}:
		arr := make([]interface{}, len(val))
		for i, vv := range val {
			arr[i] = normalizeCtxValue(vv)
		}
		return arr
	}
	return v
}

func (x *Xpr) Evaluate(expression string, context map[string]any) (any, error) {
	tokens, err := tokenize(expression)
	if err != nil {
		return nil, err
	}
	ast, err := parseTokens(tokens)
	if err != nil {
		return nil, err
	}
	vars := map[string]interface{}{}
	for k, v := range context {
		vars[k] = normalizeCtxValue(v)
	}
	ec := &evalCtx{
		vars:      vars,
		fns:       x.fns,
		depth:     0,
		startTime: time.Now(),
	}
	return evalNode(ast, ec)
}

func (x *Xpr) AddFunction(name string, fn func(...any) (any, error)) {
	x.fns[name] = xprFunc(fn)
}
