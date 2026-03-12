package xpr_test

import (
	"testing"

	xpr "github.com/xpr-lang/xpr-go"
)

func TestNotImplemented(t *testing.T) {
	engine := xpr.New()
	_, err := engine.Evaluate("1 + 2", nil)
	if err != xpr.ErrNotImplemented {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}
