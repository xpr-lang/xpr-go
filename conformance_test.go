package xpr_test

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	xpr "github.com/xpr-lang/xpr-go"
	"gopkg.in/yaml.v3"
)

type conformanceTest struct {
	Name       string                 `yaml:"name"`
	Expression string                 `yaml:"expression"`
	Context    map[string]interface{} `yaml:"context"`
	Expected   interface{}            `yaml:"expected"`
	Error      string                 `yaml:"error"`
	Tags       []string               `yaml:"tags"`
	Skip       bool                   `yaml:"skip"`
}

type conformanceSuite struct {
	Suite   string            `yaml:"suite"`
	Version string            `yaml:"version"`
	Tests   []conformanceTest `yaml:"tests"`
}

func loadConformanceSuites(t *testing.T) []conformanceSuite {
	t.Helper()
	pattern := filepath.Join("conformance", "conformance", "*.yaml")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		t.Fatalf("no conformance files found at %s", pattern)
	}
	suites := []conformanceSuite{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("failed to read %s: %v", f, err)
		}
		var suite conformanceSuite
		if err := yaml.Unmarshal(data, &suite); err != nil {
			t.Fatalf("failed to parse %s: %v", f, err)
		}
		suites = append(suites, suite)
	}
	return suites
}

func normalizeValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		if val == math.Trunc(val) && math.Abs(val) < 1e15 {
			return int64(val)
		}
		return val
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, el := range val {
			result[i] = normalizeValue(el)
		}
		return result
	case map[string]interface{}:
		result := map[string]interface{}{}
		for k, el := range val {
			result[k] = normalizeValue(el)
		}
		return result
	}
	return v
}

func toComparable(v interface{}) string {
	b, _ := json.Marshal(normalizeValue(v))
	return string(b)
}

func TestConformance(t *testing.T) {
	suites := loadConformanceSuites(t)
	engine := xpr.New()

	for _, suite := range suites {
		suite := suite
		for _, tc := range suite.Tests {
			tc := tc
			name := fmt.Sprintf("%s/%s", suite.Suite, tc.Name)
			t.Run(name, func(t *testing.T) {
				if tc.Skip {
					t.Skip("marked skip in conformance suite")
				}
				ctx := map[string]any{}
				for k, v := range tc.Context {
					ctx[k] = v
				}

				result, err := engine.Evaluate(tc.Expression, ctx)

				if tc.Error != "" {
					if err == nil {
						t.Errorf("expected error but got result: %v", result)
					}
					return
				}

				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				got := toComparable(result)
				want := toComparable(tc.Expected)
				if got != want {
					t.Errorf("\n  expr:     %s\n  expected: %s\n  got:      %s", tc.Expression, want, got)
				}
			})
		}
	}
}
