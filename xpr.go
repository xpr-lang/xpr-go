package xpr

import "errors"

// ErrNotImplemented is returned by all methods until the XPR Go runtime is implemented.
var ErrNotImplemented = errors.New("XPR Go runtime not yet implemented")

// Xpr is the XPR expression language engine.
type Xpr struct{}

// New creates a new Xpr engine instance.
func New() *Xpr { return &Xpr{} }

// Evaluate evaluates an XPR expression with the given context.
func (x *Xpr) Evaluate(expression string, context map[string]any) (any, error) {
	return nil, ErrNotImplemented
}

// AddFunction registers a custom function for use in expressions.
func (x *Xpr) AddFunction(name string, fn func(...any) (any, error)) {
	// Not yet implemented
}
