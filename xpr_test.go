package xpr_test

import (
	"testing"

	xpr "github.com/xpr-lang/xpr-go"
)

func TestBasicArithmetic(t *testing.T) {
	engine := xpr.New()
	result, err := engine.Evaluate("1 + 2", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != float64(3) {
		t.Errorf("expected 3, got %v", result)
	}
}

func TestCustomFunction(t *testing.T) {
	engine := xpr.New()
	engine.AddFunction("double", func(args ...any) (any, error) {
		return args[0].(float64) * 2, nil
	})
	result, err := engine.Evaluate("double(5)", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != float64(10) {
		t.Errorf("expected 10, got %v", result)
	}
}
