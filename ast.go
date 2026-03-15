package xpr

type nodeType int

const (
	nodeNumberLiteral nodeType = iota
	nodeStringLiteral
	nodeBooleanLiteral
	nodeNullLiteral
	nodeArrayExpression
	nodeObjectExpression
	nodeIdentifier
	nodeMemberExpression
	nodeBinaryExpression
	nodeLogicalExpression
	nodeUnaryExpression
	nodeConditionalExpression
	nodeArrowFunction
	nodeCallExpression
	nodeTemplateLiteral
	nodePipeExpression
	nodeSpreadElement
	nodeLetExpression
	nodeRegexLiteral
	nodeObjectPattern
	nodeArrayPattern
)

type node struct {
	typ      nodeType
	position int

	// NumberLiteral
	numVal float64

	// StringLiteral, BooleanLiteral (as string "true"/"false"), Identifier (name),
	// BinaryExpression/LogicalExpression/UnaryExpression (op)
	strVal string

	// BooleanLiteral
	boolVal bool

	// ArrayExpression elements, CallExpression arguments, ArrowFunction params (as []string via strSlice)
	children []*node

	// ObjectExpression properties (alternating key nodes and value nodes)
	// MemberExpression: children[0]=object, children[1]=property (if computed)
	// BinaryExpression/LogicalExpression: children[0]=left, children[1]=right
	// UnaryExpression: children[0]=argument
	// ConditionalExpression: children[0]=test, children[1]=consequent, children[2]=alternate
	// ArrowFunction: children[0]=body
	// CallExpression: children[0]=callee, children[1..]=arguments
	// TemplateLiteral: children = expressions; quasis stored in strSlice
	// PipeExpression: children[0]=left, children[1]=right

	// MemberExpression
	computed bool
	optional bool

	// MemberExpression non-computed property name, ObjectExpression property keys
	// ObjectPattern: property keys (parallel to propVals, boolSlice, defaultVals)
	// RegexLiteral: strSlice[0] = flags
	strSlice []string

	// ObjectExpression property values (parallel to strSlice)
	// ObjectPattern: binding targets (parallel to strSlice)
	propVals []*node

	// ObjectPattern/ArrayPattern: shorthand/rest flags (parallel to strSlice/children)
	boolSlice []bool

	// ObjectPattern/ArrayPattern: default value nodes (parallel to strSlice/children), nil if no default
	defaultVals []*node
}
