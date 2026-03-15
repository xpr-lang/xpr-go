package xpr

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type xprRegex struct {
	pattern string
	flags   string
	re      *regexp.Regexp
}

func destructureInto(nameNode *node, val interface{}, vars map[string]interface{}, nxt func(*node) (interface{}, error)) error {
	switch nameNode.typ {
	case nodeIdentifier:
		vars[nameNode.strVal] = val
		return nil
	case nodeObjectPattern:
		if val == nil {
			return fmt.Errorf("Cannot destructure null")
		}
		obj, _ := val.(map[string]interface{})
		if obj == nil {
			obj = map[string]interface{}{}
		}
		usedKeys := map[string]bool{}
		for i, key := range nameNode.strSlice {
			isRest := i < len(nameNode.boolSlice) && nameNode.boolSlice[i]
			if isRest {
				rest := map[string]interface{}{}
				for k, v := range obj {
					if !usedKeys[k] {
						rest[k] = v
					}
				}
				vars[key] = rest
			} else {
				usedKeys[key] = true
				v, exists := obj[key]
				if !exists {
					v = nil
				}
				if v == nil && i < len(nameNode.defaultVals) && nameNode.defaultVals[i] != nil {
					var err error
					v, err = nxt(nameNode.defaultVals[i])
					if err != nil {
						return err
					}
				}
				if err := destructureInto(nameNode.propVals[i], v, vars, nxt); err != nil {
					return err
				}
			}
		}
		return nil
	case nodeArrayPattern:
		if val == nil {
			return fmt.Errorf("Cannot destructure null")
		}
		arr, ok := val.([]interface{})
		if !ok {
			return fmt.Errorf("Cannot destructure non-array as array")
		}
		for i, el := range nameNode.children {
			isRest := i < len(nameNode.boolSlice) && nameNode.boolSlice[i]
			if isRest {
				if i < len(arr) {
					vars[el.strVal] = append([]interface{}{}, arr[i:]...)
				} else {
					vars[el.strVal] = []interface{}{}
				}
				break
			}
			var v interface{}
			if i < len(arr) {
				v = arr[i]
			}
			if v == nil && i < len(nameNode.defaultVals) && nameNode.defaultVals[i] != nil {
				var err error
				v, err = nxt(nameNode.defaultVals[i])
				if err != nil {
					return err
				}
			}
			if err := destructureInto(el, v, vars, nxt); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unexpected pattern node type %d", nameNode.typ)
}

func expandArgs(argNodes []*node, nxt func(*node) (interface{}, error)) ([]interface{}, error) {
	var result []interface{}
	for _, a := range argNodes {
		if a.typ == nodeSpreadElement {
			val, err := nxt(a.children[0])
			if err != nil {
				return nil, err
			}
			if val == nil {
				return nil, fmt.Errorf("Cannot spread null")
			}
			arr, ok := val.([]interface{})
			if !ok {
				return nil, fmt.Errorf("Cannot spread non-array")
			}
			result = append(result, arr...)
		} else {
			val, err := nxt(a)
			if err != nil {
				return nil, err
			}
			result = append(result, val)
		}
	}
	return result, nil
}

var blockedProps = map[string]bool{
	"__proto__": true, "constructor": true, "prototype": true,
	"__defineGetter__": true, "__defineSetter__": true,
	"__lookupGetter__": true, "__lookupSetter__": true,
}

const maxDepth = 50
const timeoutMs = 100

type xprFunc func(args ...interface{}) (interface{}, error)

type evalCtx struct {
	vars      map[string]interface{}
	fns       map[string]xprFunc
	depth     int
	startTime time.Time
}

func (ec *evalCtx) child(vars map[string]interface{}) *evalCtx {
	return &evalCtx{vars: vars, fns: ec.fns, depth: ec.depth + 1, startTime: ec.startTime}
}

func xprType(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch v.(type) {
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case *xprRegex:
		return "regex"
	case map[string]interface{}:
		return "object"
	}
	if _, ok := v.(xprFunc); ok {
		return "function"
	}
	return "unknown"
}

func isTruthy(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case string:
		return val != ""
	}
	return true
}

func numVal(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	}
	return 0, false
}

func formatNumber(f float64) string {
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func xprToString(v interface{}) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case bool:
		if val {
			return "true"
		}
		return "false"
	case float64:
		return formatNumber(val)
	case string:
		return val
	case *xprRegex:
		return "/" + val.pattern + "/" + val.flags
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func evalNode(n *node, ec *evalCtx) (interface{}, error) {
	if ec.depth > maxDepth {
		return nil, fmt.Errorf("expression depth limit exceeded")
	}
	if time.Since(ec.startTime).Milliseconds() > timeoutMs {
		return nil, fmt.Errorf("expression timeout exceeded")
	}

	nxt := func(child *node) (interface{}, error) {
		return evalNode(child, ec.child(ec.vars))
	}

	switch n.typ {
	case nodeNumberLiteral:
		return n.numVal, nil

	case nodeStringLiteral:
		return n.strVal, nil

	case nodeBooleanLiteral:
		return n.boolVal, nil

	case nodeNullLiteral:
		return nil, nil

	case nodeArrayExpression:
		result := []interface{}{}
		for _, el := range n.children {
			if el.typ == nodeSpreadElement {
				val, err := nxt(el.children[0])
				if err != nil {
					return nil, err
				}
				if val == nil {
					return nil, fmt.Errorf("Cannot spread null")
				}
				if _, ok := val.(string); ok {
					return nil, fmt.Errorf("Cannot spread string into array")
				}
				arr, ok := val.([]interface{})
				if !ok {
					return nil, fmt.Errorf("Cannot spread non-array into array")
				}
				result = append(result, arr...)
			} else {
				v, err := nxt(el)
				if err != nil {
					return nil, err
				}
				result = append(result, v)
			}
		}
		return result, nil

	case nodeObjectExpression:
		obj := map[string]interface{}{}
		for i, key := range n.strSlice {
			if key == "..." {
				val, err := nxt(n.propVals[i])
				if err != nil {
					return nil, err
				}
				if val == nil {
					return nil, fmt.Errorf("Cannot spread null")
				}
				if _, ok := val.([]interface{}); ok {
					return nil, fmt.Errorf("Cannot spread array into object")
				}
				m, ok := val.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("Cannot spread non-object")
				}
				for k, v := range m {
					obj[k] = v
				}
			} else {
				v, err := nxt(n.propVals[i])
				if err != nil {
					return nil, err
				}
				obj[key] = v
			}
		}
		return obj, nil

	case nodeLetExpression:
		val, err := nxt(n.children[1])
		if err != nil {
			return nil, err
		}
		childVars := make(map[string]interface{}, len(ec.vars)+4)
		for k, v := range ec.vars {
			childVars[k] = v
		}
		childEc := &evalCtx{vars: childVars, fns: ec.fns, depth: ec.depth + 1, startTime: ec.startTime}
		innerNxt := func(e *node) (interface{}, error) { return evalNode(e, childEc) }
		if err := destructureInto(n.children[0], val, childVars, innerNxt); err != nil {
			return nil, err
		}
		return evalNode(n.children[2], childEc)

	case nodeIdentifier:
		name := n.strVal
		if v, ok := ec.vars[name]; ok {
			return v, nil
		}
		if v, ok := globalConstants[name]; ok {
			return v, nil
		}
		if fn, ok := globalFunctions[name]; ok {
			return fn, nil
		}
		if fn, ok := ec.fns[name]; ok {
			return fn, nil
		}
		return nil, fmt.Errorf("unknown identifier '%s'", name)

	case nodeMemberExpression:
		obj, err := nxt(n.children[0])
		if err != nil {
			return nil, err
		}
		if n.optional && obj == nil {
			return nil, nil
		}
		if obj == nil {
			return nil, fmt.Errorf("cannot access property on null")
		}

		var propName string
		if n.computed {
			key, err := nxt(n.children[1])
			if err != nil {
				return nil, err
			}
			if f, ok := numVal(key); ok {
				arr, ok := obj.([]interface{})
				if !ok {
					return nil, fmt.Errorf("cannot index non-array with number")
				}
				idx := int(math.Trunc(f))
				if idx < 0 {
					idx = len(arr) + idx
				}
				if idx < 0 || idx >= len(arr) {
					return nil, nil
				}
				return arr[idx], nil
			}
			propName = fmt.Sprintf("%v", key)
		} else {
			propName = n.strVal
		}

		if blockedProps[propName] {
			return nil, fmt.Errorf("access denied: '%s' is a restricted property", propName)
		}

		if propName == "length" {
			if arr, ok := obj.([]interface{}); ok {
				return float64(len(arr)), nil
			}
		}

		if m, ok := obj.(map[string]interface{}); ok {
			v, exists := m[propName]
			if !exists {
				return nil, nil
			}
			return v, nil
		}

		return nil, nil

	case nodeBinaryExpression:
		left, err := nxt(n.children[0])
		if err != nil {
			return nil, err
		}
		right, err := nxt(n.children[1])
		if err != nil {
			return nil, err
		}
		op := n.strVal

		if op == "==" {
			return xprEqual(left, right), nil
		}
		if op == "!=" {
			return !xprEqual(left, right), nil
		}

		if _, ok := left.(*xprRegex); ok {
			return nil, fmt.Errorf("type error: cannot use operator '%s' with regex", op)
		}
		if _, ok := right.(*xprRegex); ok {
			return nil, fmt.Errorf("type error: cannot use operator '%s' with regex", op)
		}

		if op == "+" {
			ls, lok := left.(string)
			rs, rok := right.(string)
			if lok && rok {
				return ls + rs, nil
			}
			lf, lok := numVal(left)
			rf, rok := numVal(right)
			if lok && rok {
				return lf + rf, nil
			}
			return nil, fmt.Errorf("type error: cannot add %s and %s", xprType(left), xprType(right))
		}

		if op == "<" || op == ">" || op == "<=" || op == ">=" {
			lf, lok := numVal(left)
			rf, rok := numVal(right)
			if lok && rok {
				switch op {
				case "<":
					return lf < rf, nil
				case ">":
					return lf > rf, nil
				case "<=":
					return lf <= rf, nil
				case ">=":
					return lf >= rf, nil
				}
			}
			ls, lok := left.(string)
			rs, rok := right.(string)
			if lok && rok {
				switch op {
				case "<":
					return ls < rs, nil
				case ">":
					return ls > rs, nil
				case "<=":
					return ls <= rs, nil
				case ">=":
					return ls >= rs, nil
				}
			}
			return nil, fmt.Errorf("type error: cannot compare %s and %s", xprType(left), xprType(right))
		}

		lf, lok := numVal(left)
		rf, rok := numVal(right)
		if !lok || !rok {
			return nil, fmt.Errorf("type error: operator '%s' requires numbers, got %s and %s", op, xprType(left), xprType(right))
		}
		switch op {
		case "-":
			return lf - rf, nil
		case "*":
			return lf * rf, nil
		case "/":
			if rf == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return lf / rf, nil
		case "%":
			if rf == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return math.Mod(lf, rf), nil
		case "**":
			return math.Pow(lf, rf), nil
		}
		return nil, fmt.Errorf("unknown operator '%s'", op)

	case nodeLogicalExpression:
		left, err := nxt(n.children[0])
		if err != nil {
			return nil, err
		}
		switch n.strVal {
		case "&&":
			if isTruthy(left) {
				return nxt(n.children[1])
			}
			return left, nil
		case "||":
			if isTruthy(left) {
				return left, nil
			}
			return nxt(n.children[1])
		case "??":
			if left != nil {
				return left, nil
			}
			return nxt(n.children[1])
		}
		return nil, fmt.Errorf("unknown logical operator '%s'", n.strVal)

	case nodeUnaryExpression:
		arg, err := nxt(n.children[0])
		if err != nil {
			return nil, err
		}
		if n.strVal == "!" {
			return !isTruthy(arg), nil
		}
		if n.strVal == "-" {
			f, ok := numVal(arg)
			if !ok {
				return nil, fmt.Errorf("type error: unary minus requires number, got %s", xprType(arg))
			}
			return -f, nil
		}
		return nil, fmt.Errorf("unknown unary operator '%s'", n.strVal)

	case nodeConditionalExpression:
		test, err := nxt(n.children[0])
		if err != nil {
			return nil, err
		}
		if isTruthy(test) {
			return nxt(n.children[1])
		}
		return nxt(n.children[2])

	case nodeArrowFunction:
		capturedVars := make(map[string]interface{}, len(ec.vars))
		for k, v := range ec.vars {
			capturedVars[k] = v
		}
		restParam := n.strVal
		usePatternParams := len(n.children) > 1 || (len(n.children) == 1 && (n.children[0].typ == nodeObjectPattern || n.children[0].typ == nodeArrayPattern))
		if !usePatternParams {
			params := n.strSlice
			body := n.children[0]
			fn := xprFunc(func(args ...interface{}) (interface{}, error) {
				childVars := make(map[string]interface{}, len(capturedVars)+len(params))
				for k, v := range capturedVars {
					childVars[k] = v
				}
				for i, p := range params {
					if i < len(args) {
						childVars[p] = args[i]
					} else {
						childVars[p] = nil
					}
				}
				if restParam != "" {
					rest := make([]interface{}, 0)
					if len(args) > len(params) {
						rest = args[len(params):]
					}
					childVars[restParam] = rest
				}
				childEc := &evalCtx{vars: childVars, fns: ec.fns, depth: ec.depth + 1, startTime: ec.startTime}
				return evalNode(body, childEc)
			})
			return fn, nil
		}
		paramNodes := n.children[:len(n.children)-1]
		body := n.children[len(n.children)-1]
		fn := xprFunc(func(args ...interface{}) (interface{}, error) {
			childVars := make(map[string]interface{}, len(capturedVars)+len(paramNodes))
			for k, v := range capturedVars {
				childVars[k] = v
			}
			childEc := &evalCtx{vars: childVars, fns: ec.fns, depth: ec.depth + 1, startTime: ec.startTime}
			innerNxt := func(e *node) (interface{}, error) { return evalNode(e, childEc) }
			for i, paramNode := range paramNodes {
				var arg interface{}
				if i < len(args) {
					arg = args[i]
				}
				if err := destructureInto(paramNode, arg, childVars, innerNxt); err != nil {
					return nil, err
				}
			}
			if restParam != "" {
				rest := make([]interface{}, 0)
				if len(args) > len(paramNodes) {
					rest = args[len(paramNodes):]
				}
				childVars[restParam] = rest
			}
			return evalNode(body, childEc)
		})
		return fn, nil

	case nodeCallExpression:
		callee := n.children[0]
		argNodes := n.children[1:]

		if callee.typ == nodeMemberExpression {
			obj, err := nxt(callee.children[0])
			if err != nil {
				return nil, err
			}
			if n.optional && obj == nil {
				return nil, nil
			}
			var methodName string
			if callee.computed {
				key, err := nxt(callee.children[1])
				if err != nil {
					return nil, err
				}
				methodName = fmt.Sprintf("%v", key)
			} else {
				methodName = callee.strVal
			}
			if blockedProps[methodName] {
				return nil, fmt.Errorf("access denied: '%s' is a restricted property", methodName)
			}
			args, err := expandArgs(argNodes, nxt)
			if err != nil {
				return nil, err
			}
			if re, ok := obj.(*xprRegex); ok {
				return callRegexMethod(re, methodName, args, n.position)
			}
			return dispatchMethod(obj, methodName, args, n.position)
		}

		if callee.typ == nodeIdentifier {
			name := callee.strVal
			args, err := expandArgs(argNodes, nxt)
			if err != nil {
				return nil, err
			}
			if v, ok := ec.vars[name]; ok {
				if fn, ok := v.(xprFunc); ok {
					return fn(args...)
				}
			}
			if fn, ok := globalFunctions[name]; ok {
				arity, hasArity := globalFunctionArity[name]
				if hasArity && len(args) != arity {
					return nil, fmt.Errorf("wrong number of arguments for '%s': expected %d, got %d", name, arity, len(args))
				}
				return fn(args...)
			}
			if fn, ok := ec.fns[name]; ok {
				return fn(args...)
			}
			return nil, fmt.Errorf("unknown function '%s'", name)
		}

		calleeVal, err := nxt(callee)
		if err != nil {
			return nil, err
		}
		if n.optional && calleeVal == nil {
			return nil, nil
		}
		args, err := expandArgs(argNodes, nxt)
		if err != nil {
			return nil, err
		}
		fn, ok := calleeVal.(xprFunc)
		if !ok {
			return nil, fmt.Errorf("cannot call non-function")
		}
		return fn(args...)

	case nodePipeExpression:
		left, err := nxt(n.children[0])
		if err != nil {
			return nil, err
		}
		right := n.children[1]

		if right.typ == nodeCallExpression {
			rightCallee := right.children[0]
			rightArgNodes := right.children[1:]
			extraArgs := make([]interface{}, len(rightArgNodes))
			for i, a := range rightArgNodes {
				v, err := nxt(a)
				if err != nil {
					return nil, err
				}
				extraArgs[i] = v
			}
			if rightCallee.typ == nodeIdentifier {
				name := rightCallee.strVal
				if fn, ok := globalFunctions[name]; ok {
					allArgs := append([]interface{}{left}, extraArgs...)
					return fn(allArgs...)
				}
				if fn, ok := ec.fns[name]; ok {
					allArgs := append([]interface{}{left}, extraArgs...)
					return fn(allArgs...)
				}
				return dispatchMethod(left, name, extraArgs, n.position)
			}
			calleeVal, err := nxt(rightCallee)
			if err != nil {
				return nil, err
			}
			fn, ok := calleeVal.(xprFunc)
			if !ok {
				return nil, fmt.Errorf("pipe RHS must be callable")
			}
			allArgs := append([]interface{}{left}, extraArgs...)
			return fn(allArgs...)
		}

		if right.typ == nodeIdentifier {
			name := right.strVal
			if fn, ok := globalFunctions[name]; ok {
				return fn(left)
			}
			if fn, ok := ec.fns[name]; ok {
				return fn(left)
			}
			return dispatchMethod(left, name, nil, n.position)
		}

		rhsVal, err := nxt(right)
		if err != nil {
			return nil, err
		}
		if fn, ok := rhsVal.(xprFunc); ok {
			return fn(left)
		}
		return nil, fmt.Errorf("pipe RHS must be callable")

	case nodeTemplateLiteral:
		var sb strings.Builder
		sb.WriteString(n.strSlice[0])
		for i, expr := range n.children {
			val, err := nxt(expr)
			if err != nil {
				return nil, err
			}
			sb.WriteString(xprToString(val))
			if i+1 < len(n.strSlice) {
				sb.WriteString(n.strSlice[i+1])
			}
		}
		return sb.String(), nil

	case nodeRegexLiteral:
		pattern := n.strVal
		flags := ""
		if len(n.strSlice) > 0 {
			flags = n.strSlice[0]
		}
		prefix := ""
		if strings.Contains(flags, "i") {
			prefix += "(?i)"
		}
		if strings.Contains(flags, "m") {
			prefix += "(?m)"
		}
		if strings.Contains(flags, "s") {
			prefix += "(?s)"
		}
		re, err := regexp.Compile(prefix + pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %s", err)
		}
		return &xprRegex{pattern: pattern, flags: flags, re: re}, nil

	case nodeSpreadElement:
		return nil, fmt.Errorf("spread element used outside array context")
	}

	return nil, fmt.Errorf("unknown AST node type %d", n.typ)
}

func xprEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	ra, aIsRegex := a.(*xprRegex)
	rb, bIsRegex := b.(*xprRegex)
	if aIsRegex && bIsRegex {
		return ra.pattern == rb.pattern && ra.flags == rb.flags
	}
	if aIsRegex || bIsRegex {
		return false
	}
	_, aIsBool := a.(bool)
	_, bIsBool := b.(bool)
	if aIsBool != bIsBool {
		return false
	}
	af, aIsNum := numVal(a)
	bf, bIsNum := numVal(b)
	if aIsNum != bIsNum {
		return false
	}
	if aIsNum && bIsNum {
		return af == bf
	}
	return a == b
}

func callRegexMethod(re *xprRegex, method string, args []interface{}, pos int) (interface{}, error) {
	switch method {
	case "test":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'test': expected 1, got %d", len(args))
		}
		str, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: test expects string")
		}
		return re.re.MatchString(str), nil
	default:
		return nil, fmt.Errorf("type error: regex has no method '%s'", method)
	}
}

func dispatchMethod(obj interface{}, method string, args []interface{}, pos int) (interface{}, error) {
	switch v := obj.(type) {
	case string:
		return callStringMethod(v, method, args, pos)
	case []interface{}:
		return callArrayMethod(v, method, args, pos)
	case *xprRegex:
		return callRegexMethod(v, method, args, pos)
	case map[string]interface{}:
		return callObjectMethod(v, method, args, pos)
	}
	return nil, fmt.Errorf("type error: cannot call method '%s' on %s", method, xprType(obj))
}

func callStringMethod(s, method string, args []interface{}, pos int) (interface{}, error) {
	switch method {
	case "len":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'len': expected 0, got %d", len(args))
		}
		return float64(len([]rune(s))), nil
	case "upper":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'upper': expected 0, got %d", len(args))
		}
		return strings.ToUpper(s), nil
	case "lower":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'lower': expected 0, got %d", len(args))
		}
		return strings.ToLower(s), nil
	case "trim":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'trim': expected 0, got %d", len(args))
		}
		return strings.TrimSpace(s), nil
	case "startsWith":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'startsWith': expected 1, got %d", len(args))
		}
		prefix, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("type error: startsWith expects string argument")
		}
		return strings.HasPrefix(s, prefix), nil
	case "endsWith":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'endsWith': expected 1, got %d", len(args))
		}
		suffix, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("type error: endsWith expects string argument")
		}
		return strings.HasSuffix(s, suffix), nil
	case "contains":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'contains': expected 1, got %d", len(args))
		}
		sub, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("type error: contains expects string argument")
		}
		return strings.Contains(s, sub), nil
	case "split":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'split': expected 1, got %d", len(args))
		}
		if re, ok := args[0].(*xprRegex); ok {
			parts := re.re.Split(s, -1)
			result := make([]interface{}, len(parts))
			for i, p := range parts {
				result[i] = p
			}
			return result, nil
		}
		sep, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("type error: split expects string or regex argument")
		}
		parts := strings.Split(s, sep)
		result := make([]interface{}, len(parts))
		for i, p := range parts {
			result[i] = p
		}
		return result, nil
	case "replace":
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'replace': expected 2, got %d", len(args))
		}
		newStr, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("type error: replace replacement must be string")
		}
		if re, ok := args[0].(*xprRegex); ok {
			prefix := ""
			if strings.Contains(re.flags, "i") {
				prefix += "(?i)"
			}
			if strings.Contains(re.flags, "m") {
				prefix += "(?m)"
			}
			if strings.Contains(re.flags, "s") {
				prefix += "(?s)"
			}
			compiled, err := regexp.Compile(prefix + re.pattern)
			if err != nil {
				return nil, fmt.Errorf("invalid regex: %s", err)
			}
			return compiled.ReplaceAllString(s, newStr), nil
		}
		old, ok2 := args[0].(string)
		if !ok2 {
			return nil, fmt.Errorf("type error: replace expects string or regex as first argument")
		}
		return strings.ReplaceAll(s, old, newStr), nil
	case "match":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'match': expected 1, got %d", len(args))
		}
		re, ok := args[0].(*xprRegex)
		if !ok {
			return nil, fmt.Errorf("type error: match expects regex argument")
		}
		loc := re.re.FindStringIndex(s)
		if loc == nil {
			return nil, nil
		}
		return s[loc[0]:loc[1]], nil
	case "slice":
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'slice': expected 1-2, got %d", len(args))
		}
		startF, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: slice expects number argument")
		}
		runes := []rune(s)
		start := int(startF)
		if start < 0 {
			start = 0
		}
		if start > len(runes) {
			return "", nil
		}
		if len(args) == 2 {
			endF, ok := numVal(args[1])
			if !ok {
				return nil, fmt.Errorf("type error: slice expects number argument")
			}
			end := int(endF)
			if end > len(runes) {
				end = len(runes)
			}
			if end < start {
				return "", nil
			}
			return string(runes[start:end]), nil
		}
		return string(runes[start:]), nil
	case "indexOf":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'indexOf': expected 1, got %d", len(args))
		}
		sub, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("type error: indexOf expects string argument")
		}
		return float64(strings.Index(s, sub)), nil
	case "repeat":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'repeat': expected 1, got %d", len(args))
		}
		f, ok := numVal(args[0])
		if !ok || f != math.Trunc(f) || f < 0 {
			return nil, fmt.Errorf("type error: repeat expects non-negative integer")
		}
		return strings.Repeat(s, int(f)), nil
	case "trimStart":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'trimStart': expected 0, got %d", len(args))
		}
		return strings.TrimLeftFunc(s, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}), nil
	case "trimEnd":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'trimEnd': expected 0, got %d", len(args))
		}
		return strings.TrimRightFunc(s, func(r rune) bool {
			return r == ' ' || r == '\t' || r == '\n' || r == '\r'
		}), nil
	case "charAt":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'charAt': expected 1, got %d", len(args))
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: charAt expects number argument")
		}
		runes := []rune(s)
		idx := int(f)
		if idx < 0 || idx >= len(runes) {
			return "", nil
		}
		return string(runes[idx]), nil
	case "padStart":
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'padStart': expected 1-2, got %d", len(args))
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: padStart expects number argument")
		}
		n := int(f)
		padChar := " "
		if len(args) == 2 {
			pc, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("type error: padStart pad character must be string")
			}
			if len([]rune(pc)) > 0 {
				padChar = string([]rune(pc)[0:1])
			}
		}
		runes := []rune(s)
		if len(runes) >= n {
			return s, nil
		}
		pad := strings.Repeat(padChar, n-len(runes))
		return pad + s, nil
	case "padEnd":
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'padEnd': expected 1-2, got %d", len(args))
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: padEnd expects number argument")
		}
		n := int(f)
		padChar := " "
		if len(args) == 2 {
			pc, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("type error: padEnd pad character must be string")
			}
			if len([]rune(pc)) > 0 {
				padChar = string([]rune(pc)[0:1])
			}
		}
		runes := []rune(s)
		if len(runes) >= n {
			return s, nil
		}
		pad := strings.Repeat(padChar, n-len(runes))
		return s + pad, nil
	}
	return nil, fmt.Errorf("type error: cannot call method '%s' on string", method)
}

func callArrayMethod(arr []interface{}, method string, args []interface{}, pos int) (interface{}, error) {
	switch method {
	case "map":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'map': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'map': expected 1 function, got %d", len(args))
		}
		result := make([]interface{}, len(arr))
		for i, el := range arr {
			v, err := fn(el)
			if err != nil {
				return nil, err
			}
			result[i] = v
		}
		return result, nil

	case "filter":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'filter': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'filter': expected 1 function, got %d", len(args))
		}
		result := []interface{}{}
		for _, el := range arr {
			v, err := fn(el)
			if err != nil {
				return nil, err
			}
			if isTruthy(v) {
				result = append(result, el)
			}
		}
		return result, nil

	case "reduce":
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'reduce': expected 2 args (fn, init), got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'reduce': expected 2 args (fn, init), got %d", len(args))
		}
		acc := args[1]
		for _, el := range arr {
			v, err := fn(acc, el)
			if err != nil {
				return nil, err
			}
			acc = v
		}
		return acc, nil

	case "find":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'find': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'find': expected 1 function, got %d", len(args))
		}
		for _, el := range arr {
			v, err := fn(el)
			if err != nil {
				return nil, err
			}
			if isTruthy(v) {
				return el, nil
			}
		}
		return nil, nil

	case "some":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'some': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'some': expected 1 function, got %d", len(args))
		}
		for _, el := range arr {
			v, err := fn(el)
			if err != nil {
				return nil, err
			}
			if isTruthy(v) {
				return true, nil
			}
		}
		return false, nil

	case "every":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'every': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'every': expected 1 function, got %d", len(args))
		}
		for _, el := range arr {
			v, err := fn(el)
			if err != nil {
				return nil, err
			}
			if !isTruthy(v) {
				return false, nil
			}
		}
		return true, nil

	case "flatMap":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'flatMap': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'flatMap': expected 1 function, got %d", len(args))
		}
		result := []interface{}{}
		for _, el := range arr {
			v, err := fn(el)
			if err != nil {
				return nil, err
			}
			if sub, ok := v.([]interface{}); ok {
				result = append(result, sub...)
			} else {
				result = append(result, v)
			}
		}
		return result, nil

	case "sort":
		if len(args) > 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'sort': expected 0-1, got %d", len(args))
		}
		cp := make([]interface{}, len(arr))
		copy(cp, arr)
		var sortErr error
		if len(args) == 0 || args[0] == nil {
			sort.SliceStable(cp, func(i, j int) bool {
				a, b := cp[i], cp[j]
				af, aok := numVal(a)
				bf, bok := numVal(b)
				if aok && bok {
					return af < bf
				}
				return fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b)
			})
		} else {
			fn, ok := args[0].(xprFunc)
			if !ok {
				return nil, fmt.Errorf("type error: sort expects function argument")
			}
			sort.SliceStable(cp, func(i, j int) bool {
				if sortErr != nil {
					return false
				}
				v, err := fn(cp[i], cp[j])
				if err != nil {
					sortErr = err
					return false
				}
				if f, ok := numVal(v); ok {
					return f < 0
				}
				return false
			})
		}
		if sortErr != nil {
			return nil, sortErr
		}
		return cp, nil

	case "reverse":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'reverse': expected 0, got %d", len(args))
		}
		cp := make([]interface{}, len(arr))
		for i, v := range arr {
			cp[len(arr)-1-i] = v
		}
		return cp, nil
	case "includes":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'includes': expected 1, got %d", len(args))
		}
		for _, el := range arr {
			if xprEqual(el, args[0]) {
				return true, nil
			}
		}
		return false, nil
	case "indexOf":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'indexOf': expected 1, got %d", len(args))
		}
		for i, el := range arr {
			if xprEqual(el, args[0]) {
				return float64(i), nil
			}
		}
		return float64(-1), nil
	case "slice":
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'slice': expected 1-2, got %d", len(args))
		}
		startF, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: slice expects number argument")
		}
		start := int(startF)
		if start < 0 {
			start = 0
		}
		if start > len(arr) {
			return []interface{}{}, nil
		}
		if len(args) == 2 {
			endF, ok := numVal(args[1])
			if !ok {
				return nil, fmt.Errorf("type error: slice expects number argument")
			}
			end := int(endF)
			if end > len(arr) {
				end = len(arr)
			}
			if end < start {
				return []interface{}{}, nil
			}
			return arr[start:end], nil
		}
		return arr[start:], nil
	case "join":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'join': expected 1, got %d", len(args))
		}
		sep, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("type error: join expects string argument")
		}
		parts := make([]string, len(arr))
		for i, el := range arr {
			parts[i] = xprToString(el)
		}
		return strings.Join(parts, sep), nil
	case "concat":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'concat': expected 1, got %d", len(args))
		}
		other, ok := args[0].([]interface{})
		if !ok {
			return nil, fmt.Errorf("type error: concat expects array argument")
		}
		result := make([]interface{}, len(arr)+len(other))
		copy(result, arr)
		copy(result[len(arr):], other)
		return result, nil
	case "flat":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'flat': expected 0, got %d", len(args))
		}
		result := []interface{}{}
		for _, el := range arr {
			if sub, ok := el.([]interface{}); ok {
				result = append(result, sub...)
			} else {
				result = append(result, el)
			}
		}
		return result, nil
	case "unique":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'unique': expected 0, got %d", len(args))
		}
		seen := []interface{}{}
		result := []interface{}{}
		for _, el := range arr {
			found := false
			for _, s := range seen {
				if xprEqual(el, s) {
					found = true
					break
				}
			}
			if !found {
				seen = append(seen, el)
				result = append(result, el)
			}
		}
		return result, nil
	case "zip":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'zip': expected 1, got %d", len(args))
		}
		other, ok := args[0].([]interface{})
		if !ok {
			return nil, fmt.Errorf("type error: zip expects array argument")
		}
		length := len(arr)
		if len(other) < length {
			length = len(other)
		}
		result := make([]interface{}, length)
		for i := 0; i < length; i++ {
			result[i] = []interface{}{arr[i], other[i]}
		}
		return result, nil
	case "chunk":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'chunk': expected 1, got %d", len(args))
		}
		f, ok := numVal(args[0])
		if !ok || f != math.Trunc(f) || f <= 0 {
			return nil, fmt.Errorf("type error: chunk size must be a positive integer")
		}
		size := int(f)
		result := []interface{}{}
		for i := 0; i < len(arr); i += size {
			end := i + size
			if end > len(arr) {
				end = len(arr)
			}
			chunk := make([]interface{}, end-i)
			copy(chunk, arr[i:end])
			result = append(result, chunk)
		}
		return result, nil
	case "groupBy":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'groupBy': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'groupBy': expected 1 function, got %d", len(args))
		}
		groups := map[string][]interface{}{}
		groupKeys := []string{}
		for _, el := range arr {
			keyVal, err := fn(el)
			if err != nil {
				return nil, err
			}
			key := fmt.Sprintf("%v", keyVal)
			if _, exists := groups[key]; !exists {
				groupKeys = append(groupKeys, key)
			}
			groups[key] = append(groups[key], el)
		}
		sort.Strings(groupKeys)
		result := map[string]interface{}{}
		for _, k := range groupKeys {
			result[k] = groups[k]
		}
		return result, nil

	case "sortBy":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'sortBy': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'sortBy': expected 1 function")
		}
		type indexedEl struct {
			idx int
			key interface{}
			val interface{}
		}
		indexed := make([]indexedEl, len(arr))
		allNumbers := true
		allStrings := true
		for i, el := range arr {
			k, err := fn(el)
			if err != nil {
				return nil, err
			}
			if _, isNum := numVal(k); !isNum {
				allNumbers = false
			}
			if _, isStr := k.(string); !isStr {
				allStrings = false
			}
			indexed[i] = indexedEl{idx: i, key: k, val: el}
		}
		if len(arr) > 0 && !allNumbers && !allStrings {
			return nil, fmt.Errorf("type error: sortBy key function must return all numbers or all strings")
		}
		sort.SliceStable(indexed, func(i, j int) bool {
			if allNumbers {
				a, _ := numVal(indexed[i].key)
				b, _ := numVal(indexed[j].key)
				return a < b
			}
			return indexed[i].key.(string) < indexed[j].key.(string)
		})
		out := make([]interface{}, len(arr))
		for i, el := range indexed {
			out[i] = el.val
		}
		return out, nil

	case "take":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'take': expected 1, got %d", len(args))
		}
		nf, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: take expects integer argument")
		}
		n := int(nf)
		if n <= 0 {
			return []interface{}{}, nil
		}
		if n > len(arr) {
			n = len(arr)
		}
		out := make([]interface{}, n)
		copy(out, arr[:n])
		return out, nil

	case "drop":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'drop': expected 1, got %d", len(args))
		}
		nf, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: drop expects integer argument")
		}
		n := int(nf)
		if n <= 0 {
			out := make([]interface{}, len(arr))
			copy(out, arr)
			return out, nil
		}
		if n >= len(arr) {
			return []interface{}{}, nil
		}
		out := make([]interface{}, len(arr)-n)
		copy(out, arr[n:])
		return out, nil

	case "count":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'count': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'count': expected 1 function")
		}
		count := 0.0
		for _, el := range arr {
			v, err := fn(el)
			if err != nil {
				return nil, err
			}
			if isTruthy(v) {
				count++
			}
		}
		return count, nil

	case "sum":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'sum': expected 0, got %d", len(args))
		}
		if len(arr) == 0 {
			return 0.0, nil
		}
		total := 0.0
		for _, el := range arr {
			f, ok := numVal(el)
			if !ok {
				return nil, fmt.Errorf("type error: sum expects all elements to be numbers, got %s", xprType(el))
			}
			total += f
		}
		return total, nil

	case "avg":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'avg': expected 0, got %d", len(args))
		}
		if len(arr) == 0 {
			return nil, fmt.Errorf("type error: cannot compute average of empty array")
		}
		total := 0.0
		for _, el := range arr {
			f, ok := numVal(el)
			if !ok {
				return nil, fmt.Errorf("type error: avg expects all elements to be numbers, got %s", xprType(el))
			}
			total += f
		}
		return total / float64(len(arr)), nil

	case "compact":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'compact': expected 0, got %d", len(args))
		}
		out := []interface{}{}
		for _, el := range arr {
			if el != nil {
				out = append(out, el)
			}
		}
		return out, nil

	case "partition":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'partition': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'partition': expected 1 function")
		}
		matches := []interface{}{}
		nonMatches := []interface{}{}
		for _, el := range arr {
			v, err := fn(el)
			if err != nil {
				return nil, err
			}
			if isTruthy(v) {
				matches = append(matches, el)
			} else {
				nonMatches = append(nonMatches, el)
			}
		}
		return []interface{}{matches, nonMatches}, nil

	case "keyBy":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'keyBy': expected 1 function, got %d", len(args))
		}
		fn, ok := args[0].(xprFunc)
		if !ok {
			return nil, fmt.Errorf("wrong number of arguments for 'keyBy': expected 1 function")
		}
		kbResult := map[string]interface{}{}
		kbKeys := []string{}
		for _, el := range arr {
			keyVal, err := fn(el)
			if err != nil {
				return nil, err
			}
			key := fmt.Sprintf("%v", keyVal)
			if _, exists := kbResult[key]; !exists {
				kbKeys = append(kbKeys, key)
			}
			kbResult[key] = el
		}
		sort.Strings(kbKeys)
		sorted := map[string]interface{}{}
		for _, k := range kbKeys {
			sorted[k] = kbResult[k]
		}
		return sorted, nil

	case "min":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'min': expected 0, got %d", len(args))
		}
		if len(arr) == 0 {
			return nil, fmt.Errorf("type error: cannot compute min of empty array")
		}
		minVal, ok := numVal(arr[0])
		if !ok {
			return nil, fmt.Errorf("type error: min expects all elements to be numbers, got %s", xprType(arr[0]))
		}
		for _, el := range arr[1:] {
			f, ok := numVal(el)
			if !ok {
				return nil, fmt.Errorf("type error: min expects all elements to be numbers, got %s", xprType(el))
			}
			if f < minVal {
				minVal = f
			}
		}
		return minVal, nil

	case "max":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'max': expected 0, got %d", len(args))
		}
		if len(arr) == 0 {
			return nil, fmt.Errorf("type error: cannot compute max of empty array")
		}
		maxVal, ok := numVal(arr[0])
		if !ok {
			return nil, fmt.Errorf("type error: max expects all elements to be numbers, got %s", xprType(arr[0]))
		}
		for _, el := range arr[1:] {
			f, ok := numVal(el)
			if !ok {
				return nil, fmt.Errorf("type error: max expects all elements to be numbers, got %s", xprType(el))
			}
			if f > maxVal {
				maxVal = f
			}
		}
		return maxVal, nil

	case "first":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'first': expected 0, got %d", len(args))
		}
		if len(arr) == 0 {
			return nil, nil
		}
		return arr[0], nil

	case "last":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'last': expected 0, got %d", len(args))
		}
		if len(arr) == 0 {
			return nil, nil
		}
		return arr[len(arr)-1], nil
	}
	return nil, fmt.Errorf("type error: cannot call method '%s' on array", method)
}

func callObjectMethod(obj map[string]interface{}, method string, args []interface{}, pos int) (interface{}, error) {
	switch method {
	case "keys":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'keys': expected 0, got %d", len(args))
		}
		rawKeys := make([]string, 0, len(obj))
		for k := range obj {
			rawKeys = append(rawKeys, k)
		}
		sort.Strings(rawKeys)
		keys := make([]interface{}, len(rawKeys))
		for i, k := range rawKeys {
			keys[i] = k
		}
		return keys, nil
	case "values":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'values': expected 0, got %d", len(args))
		}
		rawKeys := make([]string, 0, len(obj))
		for k := range obj {
			rawKeys = append(rawKeys, k)
		}
		sort.Strings(rawKeys)
		vals := make([]interface{}, len(rawKeys))
		for i, k := range rawKeys {
			vals[i] = obj[k]
		}
		return vals, nil
	case "entries":
		if len(args) != 0 {
			return nil, fmt.Errorf("wrong number of arguments for 'entries': expected 0, got %d", len(args))
		}
		rawKeys := make([]string, 0, len(obj))
		for k := range obj {
			rawKeys = append(rawKeys, k)
		}
		sort.Strings(rawKeys)
		result := make([]interface{}, len(rawKeys))
		for i, k := range rawKeys {
			result[i] = []interface{}{k, obj[k]}
		}
		return result, nil
	case "has":
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'has': expected 1, got %d", len(args))
		}
		key, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("type error: has expects string argument")
		}
		_, exists := obj[key]
		return exists, nil
	}
	return nil, fmt.Errorf("type error: cannot call method '%s' on object", method)
}

func icuToGoFormat(icu string) string {
	result := icu
	result = strings.ReplaceAll(result, "yyyy", "2006")
	result = strings.ReplaceAll(result, "MM", "01")
	result = strings.ReplaceAll(result, "dd", "02")
	result = strings.ReplaceAll(result, "HH", "15")
	result = strings.ReplaceAll(result, "mm", "04")
	result = strings.ReplaceAll(result, "ss", "05")
	result = strings.ReplaceAll(result, "SSS", "000")
	return result
}

func epochMsToTime(args []interface{}, funcName string) (time.Time, error) {
	if len(args) != 1 {
		return time.Time{}, fmt.Errorf("wrong number of arguments for '%s': expected 1, got %d", funcName, len(args))
	}
	ms, ok := numVal(args[0])
	if !ok {
		return time.Time{}, fmt.Errorf("Type error: %s expects number (epoch ms)", funcName)
	}
	return time.UnixMilli(int64(ms)).UTC(), nil
}

func extractInlineFlags(pattern string) (string, string) {
	re := regexp.MustCompile(`^\(\?([imsu]+)\)(.*)`)
	m := re.FindStringSubmatch(pattern)
	if m != nil {
		return m[2], m[1]
	}
	return pattern, ""
}

func compileWithFlags(pattern string) (*regexp.Regexp, error) {
	src, flags := extractInlineFlags(pattern)
	prefix := ""
	if strings.Contains(flags, "i") {
		prefix += "(?i)"
	}
	if strings.Contains(flags, "m") {
		prefix += "(?m)"
	}
	if strings.Contains(flags, "s") {
		prefix += "(?s)"
	}
	return regexp.Compile(prefix + src)
}

var globalConstants = map[string]interface{}{
	"PI": math.Pi,
	"E":  math.E,
}

var globalFunctionArity = map[string]int{
	"round": 1, "floor": 1, "ceil": 1, "abs": 1,
	"type": 1, "int": 1, "float": 1, "string": 1, "bool": 1,
	"sqrt": 1, "log": 1, "sign": 1, "trunc": 1,
	"isNumber": 1, "isString": 1, "isArray": 1, "isNull": 1, "isObject": 1, "isRegex": 1,
	"fromEntries": 1,
	"pow":         2, "random": 0,
}

var globalFunctions = map[string]xprFunc{
	"round": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'round'")
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: round expects number")
		}
		return math.Round(f), nil
	},
	"floor": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'floor'")
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: floor expects number")
		}
		return math.Floor(f), nil
	},
	"ceil": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'ceil'")
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: ceil expects number")
		}
		return math.Ceil(f), nil
	},
	"abs": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'abs'")
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: abs expects number")
		}
		return math.Abs(f), nil
	},
	"min": func(args ...interface{}) (interface{}, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'min': expected at least 2, got %d", len(args))
		}
		result, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: min expects numbers")
		}
		for _, a := range args[1:] {
			f, ok := numVal(a)
			if !ok {
				return nil, fmt.Errorf("type error: min expects numbers")
			}
			result = math.Min(result, f)
		}
		return result, nil
	},
	"max": func(args ...interface{}) (interface{}, error) {
		if len(args) < 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'max': expected at least 2, got %d", len(args))
		}
		result, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("type error: max expects numbers")
		}
		for _, a := range args[1:] {
			f, ok := numVal(a)
			if !ok {
				return nil, fmt.Errorf("type error: max expects numbers")
			}
			result = math.Max(result, f)
		}
		return result, nil
	},
	"type": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'type'")
		}
		return xprType(args[0]), nil
	},
	"int": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'int'")
		}
		switch v := args[0].(type) {
		case bool:
			return nil, fmt.Errorf("type error: cannot convert boolean to int")
		case float64:
			return math.Trunc(v), nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("type error: cannot convert \"%s\" to int", v)
			}
			return math.Trunc(f), nil
		}
		return nil, fmt.Errorf("type error: cannot convert %s to int", xprType(args[0]))
	},
	"float": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'float'")
		}
		switch v := args[0].(type) {
		case bool:
			return nil, fmt.Errorf("type error: cannot convert boolean to float")
		case float64:
			return v, nil
		case string:
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return nil, fmt.Errorf("type error: cannot convert \"%s\" to float", v)
			}
			return f, nil
		}
		return nil, fmt.Errorf("type error: cannot convert %s to float", xprType(args[0]))
	},
	"string": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'string'")
		}
		return xprToString(args[0]), nil
	},
	"bool": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'bool'")
		}
		return isTruthy(args[0]), nil
	},
	"range": func(args ...interface{}) (interface{}, error) {
		var start, end, step float64
		switch len(args) {
		case 1:
			f, ok := numVal(args[0])
			if !ok {
				return nil, fmt.Errorf("type error: range expects number arguments")
			}
			start, end, step = 0, f, 1
		case 2:
			f0, ok0 := numVal(args[0])
			f1, ok1 := numVal(args[1])
			if !ok0 || !ok1 {
				return nil, fmt.Errorf("type error: range expects number arguments")
			}
			start, end, step = f0, f1, 1
		case 3:
			f0, ok0 := numVal(args[0])
			f1, ok1 := numVal(args[1])
			f2, ok2 := numVal(args[2])
			if !ok0 || !ok1 || !ok2 {
				return nil, fmt.Errorf("type error: range expects number arguments")
			}
			start, end, step = f0, f1, f2
		default:
			return nil, fmt.Errorf("wrong number of arguments for 'range': expected 1-3, got %d", len(args))
		}
		if step != math.Trunc(step) {
			return nil, fmt.Errorf("type error: range step must be an integer, got float")
		}
		if step == 0 {
			return nil, fmt.Errorf("type error: range step cannot be zero")
		}
		result := []interface{}{}
		if step > 0 {
			for i := start; i < end; i += step {
				result = append(result, i)
			}
		} else {
			for i := start; i > end; i += step {
				result = append(result, i)
			}
		}
		return result, nil
	},

	// ── Date/Time Functions (v0.3) ──────────────────────────────────────────
	"now": func(args ...interface{}) (interface{}, error) {
		return float64(time.Now().UTC().UnixMilli()), nil
	},
	"parseDate": func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'parseDate': expected 1-2, got %d", len(args))
		}
		str, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: parseDate expects string")
		}
		if len(args) == 1 {
			formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02T15:04:05", "2006-01-02"}
			for _, fmt2 := range formats {
				if t, err := time.ParseInLocation(fmt2, str, time.UTC); err == nil {
					return float64(t.UnixMilli()), nil
				}
			}
			return nil, fmt.Errorf("invalid date string: %q", str)
		}
		fmtStr, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: parseDate format must be string")
		}
		goFmt := icuToGoFormat(fmtStr)
		t, err := time.ParseInLocation(goFmt, str, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("invalid date string: %q does not match format %q", str, fmtStr)
		}
		return float64(t.UnixMilli()), nil
	},
	"formatDate": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'formatDate': expected 2, got %d", len(args))
		}
		ms, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("Type error: formatDate expects number (epoch ms)")
		}
		fmtStr, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: formatDate format must be string")
		}
		t := time.UnixMilli(int64(ms)).UTC()
		return t.Format(icuToGoFormat(fmtStr)), nil
	},
	"year": func(args ...interface{}) (interface{}, error) {
		t, err := epochMsToTime(args, "year")
		if err != nil {
			return nil, err
		}
		return float64(t.Year()), nil
	},
	"month": func(args ...interface{}) (interface{}, error) {
		t, err := epochMsToTime(args, "month")
		if err != nil {
			return nil, err
		}
		return float64(t.Month()), nil
	},
	"day": func(args ...interface{}) (interface{}, error) {
		t, err := epochMsToTime(args, "day")
		if err != nil {
			return nil, err
		}
		return float64(t.Day()), nil
	},
	"hour": func(args ...interface{}) (interface{}, error) {
		t, err := epochMsToTime(args, "hour")
		if err != nil {
			return nil, err
		}
		return float64(t.Hour()), nil
	},
	"minute": func(args ...interface{}) (interface{}, error) {
		t, err := epochMsToTime(args, "minute")
		if err != nil {
			return nil, err
		}
		return float64(t.Minute()), nil
	},
	"second": func(args ...interface{}) (interface{}, error) {
		t, err := epochMsToTime(args, "second")
		if err != nil {
			return nil, err
		}
		return float64(t.Second()), nil
	},
	"millisecond": func(args ...interface{}) (interface{}, error) {
		t, err := epochMsToTime(args, "millisecond")
		if err != nil {
			return nil, err
		}
		return float64(t.Nanosecond() / 1e6), nil
	},
	"dateAdd": func(args ...interface{}) (interface{}, error) {
		if len(args) != 3 {
			return nil, fmt.Errorf("wrong number of arguments for 'dateAdd': expected 3, got %d", len(args))
		}
		ms, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("Type error: dateAdd expects number (epoch ms)")
		}
		amount, ok := numVal(args[1])
		if !ok {
			return nil, fmt.Errorf("Type error: dateAdd amount must be number")
		}
		unit, ok := args[2].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: dateAdd unit must be string")
		}
		amt := int(math.Trunc(amount))
		t := time.UnixMilli(int64(ms)).UTC()
		switch unit {
		case "years":
			t = t.AddDate(amt, 0, 0)
		case "months":
			t = t.AddDate(0, amt, 0)
		case "days":
			t = t.AddDate(0, 0, amt)
		case "hours":
			t = t.Add(time.Duration(amt) * time.Hour)
		case "minutes":
			t = t.Add(time.Duration(amt) * time.Minute)
		case "seconds":
			t = t.Add(time.Duration(amt) * time.Second)
		case "milliseconds":
			return ms + float64(amt), nil
		default:
			return nil, fmt.Errorf("invalid unit %q for dateAdd", unit)
		}
		return float64(t.UnixMilli()), nil
	},
	"dateDiff": func(args ...interface{}) (interface{}, error) {
		if len(args) != 3 {
			return nil, fmt.Errorf("wrong number of arguments for 'dateDiff': expected 3, got %d", len(args))
		}
		ms1, ok1 := numVal(args[0])
		ms2, ok2 := numVal(args[1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("Type error: dateDiff expects number (epoch ms)")
		}
		unit, ok := args[2].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: dateDiff unit must be string")
		}
		diffMs := ms2 - ms1
		switch unit {
		case "milliseconds":
			return diffMs, nil
		case "seconds":
			return math.Trunc(diffMs / 1000), nil
		case "minutes":
			return math.Trunc(diffMs / 60000), nil
		case "hours":
			return math.Trunc(diffMs / 3600000), nil
		case "days":
			return math.Trunc(diffMs / 86400000), nil
		case "months":
			t1 := time.UnixMilli(int64(ms1)).UTC()
			t2 := time.UnixMilli(int64(ms2)).UTC()
			return float64((t2.Year()-t1.Year())*12 + int(t2.Month()-t1.Month())), nil
		case "years":
			t1 := time.UnixMilli(int64(ms1)).UTC()
			t2 := time.UnixMilli(int64(ms2)).UTC()
			return float64(t2.Year() - t1.Year()), nil
		default:
			return nil, fmt.Errorf("invalid unit %q for dateDiff", unit)
		}
	},

	// ── Regex Functions (v0.3) ────────────────────────────────────────────────
	"matches": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'matches': expected 2, got %d", len(args))
		}
		str, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: matches expects string")
		}
		pattern, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: matches pattern must be string")
		}
		re, err := compileWithFlags(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %s", err)
		}
		return re.MatchString(str), nil
	},
	"match": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'match': expected 2, got %d", len(args))
		}
		str, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: match expects string")
		}
		pattern, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: match pattern must be string")
		}
		re, err := compileWithFlags(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %s", err)
		}
		loc := re.FindStringIndex(str)
		if loc == nil {
			return nil, nil
		}
		return str[loc[0]:loc[1]], nil
	},
	"matchAll": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'matchAll': expected 2, got %d", len(args))
		}
		str, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: matchAll expects string")
		}
		pattern, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: matchAll pattern must be string")
		}
		re, err := compileWithFlags(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %s", err)
		}
		matches := re.FindAllString(str, -1)
		result := make([]interface{}, len(matches))
		for i, m := range matches {
			result[i] = m
		}
		return result, nil
	},
	"replacePattern": func(args ...interface{}) (interface{}, error) {
		if len(args) != 3 {
			return nil, fmt.Errorf("wrong number of arguments for 'replacePattern': expected 3, got %d", len(args))
		}
		str, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: replacePattern expects string")
		}
		pattern, ok := args[1].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: replacePattern pattern must be string")
		}
		replacement, ok := args[2].(string)
		if !ok {
			return nil, fmt.Errorf("Type error: replacePattern replacement must be string")
		}
		re, err := compileWithFlags(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex pattern: %s", err)
		}
		goRepl := regexp.MustCompile(`\$(\d+)`).ReplaceAllString(replacement, "$${${1}}")
		return re.ReplaceAllString(str, goRepl), nil
	},

	"sqrt": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'sqrt'")
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("Type error: sqrt expects number")
		}
		if f < 0 {
			return nil, fmt.Errorf("Type error: cannot compute sqrt of negative number")
		}
		return math.Sqrt(f), nil
	},
	"log": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'log'")
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("Type error: log expects number")
		}
		if f <= 0 {
			return nil, fmt.Errorf("Type error: cannot compute log of non-positive number")
		}
		return math.Log(f), nil
	},
	"pow": func(args ...interface{}) (interface{}, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("wrong number of arguments for 'pow': expected 2, got %d", len(args))
		}
		x, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("Type error: pow expects number")
		}
		y, ok := numVal(args[1])
		if !ok {
			return nil, fmt.Errorf("Type error: pow expects number")
		}
		return math.Pow(x, y), nil
	},
	"random": func(args ...interface{}) (interface{}, error) {
		return rand.Float64(), nil
	},
	"sign": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'sign'")
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("Type error: sign expects number")
		}
		if f > 0 {
			return 1.0, nil
		}
		if f < 0 {
			return -1.0, nil
		}
		return 0.0, nil
	},
	"trunc": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'trunc'")
		}
		f, ok := numVal(args[0])
		if !ok {
			return nil, fmt.Errorf("Type error: trunc expects number")
		}
		return math.Trunc(f), nil
	},

	"isNumber": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'isNumber'")
		}
		_, ok := numVal(args[0])
		return ok, nil
	},
	"isString": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'isString'")
		}
		_, ok := args[0].(string)
		return ok, nil
	},
	"isArray": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'isArray'")
		}
		_, ok := args[0].([]interface{})
		return ok, nil
	},
	"isNull": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'isNull'")
		}
		return args[0] == nil, nil
	},
	"isObject": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'isObject'")
		}
		if _, isArr := args[0].([]interface{}); isArr {
			return false, nil
		}
		if _, isRe := args[0].(*xprRegex); isRe {
			return false, nil
		}
		if args[0] == nil {
			return false, nil
		}
		_, ok := args[0].(map[string]interface{})
		return ok, nil
	},
	"isRegex": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'isRegex'")
		}
		_, ok := args[0].(*xprRegex)
		return ok, nil
	},

	"fromEntries": func(args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("wrong number of arguments for 'fromEntries': expected 1, got %d", len(args))
		}
		pairs, ok := args[0].([]interface{})
		if !ok {
			return nil, fmt.Errorf("Type error: fromEntries expects array")
		}
		result := map[string]interface{}{}
		keys := []string{}
		for _, pair := range pairs {
			pairArr, ok := pair.([]interface{})
			if !ok || len(pairArr) < 2 {
				return nil, fmt.Errorf("Type error: fromEntries each element must be [key, value] pair")
			}
			var key string
			if f, ok := numVal(pairArr[0]); ok {
				if f == math.Trunc(f) {
					key = strconv.FormatInt(int64(f), 10)
				} else {
					key = strconv.FormatFloat(f, 'f', -1, 64)
				}
			} else {
				key = fmt.Sprintf("%v", pairArr[0])
			}
			if _, exists := result[key]; !exists {
				keys = append(keys, key)
			}
			result[key] = pairArr[1]
		}
		sort.Strings(keys)
		sorted := map[string]interface{}{}
		for _, k := range keys {
			sorted[k] = result[k]
		}
		return sorted, nil
	},
}
