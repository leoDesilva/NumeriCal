package evaluator

import (
	"errors"
	"math"
	"numerical/lexer"
	"numerical/parser"
	"strings"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
	units "github.com/bcicen/go-units"
)

/* ----------------------------- Define Go Units ---------------------------- */

func DefineUnits() {
	Week := units.NewUnit("week", "weeks")
	units.NewRatioConversion(Week, units.Day, 7.0)
}

/* --------------------------- Evaluator Functions -------------------------- */

func Eval(node parser.Node, environment Environment) (Object, error) {
	switch n := node.(type) {
	case *parser.ProgramNode:
		program := Program{}
		for _, node := range n.Nodes {
			result, err := Eval(node, environment)
			if err != nil {
				return &Error{}, err
			}
			program.Objects = append(program.Objects, result)
		}
		return &program, nil

	case *parser.UnaryOpNode:
		return handleReturn(evalUnaryOp(n, environment))

	case *parser.BinOpNode:
		return handleReturn(evalBinaryOp(n, environment))

	case *parser.UnitNode:
		value, err := Eval(n.Value, environment)
		if err != nil {
			return &Error{}, err
		}
		objectValue, objErr := isNumberObject(value)
		return handleReturn(&Unit{Value: objectValue.Inspect(), Unit: n.Unit}, objErr)

	case *parser.AssignNode:
		value, err := Eval(n.Expression, environment)
		if err != nil {
			return &Error{}, err
		}
		environment.Variables[n.Identifier] = value
		return &Nil{}, nil

	case *parser.FunctionCallNode:
		return evalFunctionCall(n, environment)

	case *parser.FunctionDefenitionNode:
		environment.Functions[n.Identifier] = n
		return &Nil{}, nil

	case *parser.IdentifierNode:
		if value, ok := environment.Constants[n.Identifier]; ok {
			return value, nil
		}

		element, err := lookupElements(n.Identifier, environment.PeriodicTable)
		if err == nil {
			return formatFloat(element["atomic_mass"].(float64)), nil
		}

		if value, ok := environment.Variables[n.Identifier]; ok {
			return value, nil
		} else {
			if len(environment.Variables) < 1 {
				return &Error{}, errors.New("VarAccessError: Undefined variable identifier " + n.Identifier)
			}

			maxIdentifier := ""
			maxSimilarity := 0.0

			for variable := range environment.Variables {
				similarity := similarity(n.Identifier, variable)
				if similarity > maxSimilarity {
					maxSimilarity = similarity
					maxIdentifier = variable
				}
			}

			return environment.Variables[maxIdentifier], nil
		}

	case *parser.IntNode:
		return &Integer{Value: n.Value}, nil

	case *parser.FloatNode:
		return &Float{Value: n.Value}, nil

	case *parser.StringNode:
		return &String{Value: n.Value}, nil
	}

	return &Error{}, nil
}

func evalFunctionCall(n *parser.FunctionCallNode, environment Environment) (Object, error) {
	var functions = map[string]func(Program, Environment) (Object, error){
		"frac":   frac,
		"print":  print,
		"root":   root,
		"lookup": lookup,
	}

	if function, ok := functions[n.Identifier]; ok {
		params, err := Eval(&n.Parameters, environment)
		if err != nil {
			return &Error{}, err
		}
		if paramsProgram, ok := params.(*Program); ok {
			result, err := function(*paramsProgram, environment)
			if err != nil {
				return &Error{}, err
			}
			return result, nil
		}
	} else if function, ok := environment.Functions[n.Identifier]; ok {
		env := Environment{Variables: make(map[string]Object), Functions: make(map[string]*parser.FunctionDefenitionNode)}
		for i, node := range n.Parameters.Nodes {
			identifer := function.Parameters[i].(*parser.IdentifierNode).Identifier
			result, err := Eval(node, environment)
			if err != nil {
				return &Error{}, err
			}
			env.Variables[identifer] = result
		}
		result, err := Eval(&function.Consequence, env)
		if err != nil {
			return &Error{}, err
		}
		return result.(*Program).Objects[len(result.(*Program).Objects)-1], nil
	}

	return &Error{}, errors.New("FunctionCallError: Function with Identifer " + n.Identifier + " is not defined")
}

/* ---------------------------- Unary Operations ---------------------------- */

func evalUnaryOp(node *parser.UnaryOpNode, environment Environment) (Object, error) {
	result, err := Eval(node.Right, environment)
	if err != nil {
		return &Error{}, err
	}

	switch node.Operation {
	case lexer.SUB:
		return evalUnarySub(result)
	case lexer.NOT:
		return evalUnaryNot(result), nil
	case lexer.TILDE:
		return evalUnaryRound(result)
	}

	return &Error{}, errors.New("UnaryOperationError: Unsupported " + node.Operation + " Operation")
}

func evalUnarySub(node Object) (Object, error) {
	switch n := node.(type) {
	case *Integer:
		return &Integer{Value: -n.Value}, nil
	case *Float:
		return &Float{Value: -n.Value}, nil
	}

	return &Error{}, errors.New("UnaryOperationError: Cannot negate type " + node.Type())
}

func evalUnaryRound(node Object) (Object, error) {
	switch n := node.(type) {
	case *Integer:
		return n, nil
	case *Float:
		return &Integer{Value: int(math.Round(n.Value))}, nil
	}

	return &Error{}, errors.New("RoundingError: cannout round type " + node.Type())
}

func evalUnaryNot(node Object) *Integer {
	switch n := node.(type) {
	case *Integer:
		if n.Value == 0 {
			return &Integer{Value: 1}
		}
	case *String:
		if n.Value == "" {
			return &Integer{Value: 1}
		}
	}

	return &Integer{Value: 0}
}

/* ---------------------------- Binary Operations --------------------------- */

func evalBinaryOp(node *parser.BinOpNode, environment Environment) (Object, error) {
	left, err := Eval(node.Left, environment)
	if err != nil {
		return &Error{}, err
	}

	if node.Operation == lexer.IN && node.Right.Type() == lexer.IDENTIFIER_NODE {
		toIdentifier := node.Right.(*parser.IdentifierNode).Identifier

		if leftUnit, ok := left.(*Unit); ok {
			return convert(leftUnit.Inspect(), leftUnit.Unit, toIdentifier)

		} else if leftUnit, ok := left.(Number); ok {
			return convert(leftUnit.Inspect(), toIdentifier, toIdentifier)
		}

	} else if node.Operation == lexer.IN && node.Right.Type() != lexer.IDENTIFIER_NODE {
		return &Error{}, errors.New("ConversionError: IN cannot convert " + left.Type() + " and " + node.Right.Type())
	}

	right, err := Eval(node.Right, environment)
	if err != nil {
		return &Error{}, err
	}

	switch left := left.(type) {
	case Number:
		if right, ok := right.(Number); ok {
			return handleReturn(evalNumberInfix(left, right, node.Operation))
		}

	case *String:
		if right.Type() == lexer.STRING_OBJ {
			return handleReturn(evalStringInfix(left, right.(*String), node.Operation))
		}

		//TODO Array Node + possibly others such as matrix
	}

	return &Error{}, errors.New("BinaryOperationError: Unsupported Types: " + left.Type() + node.Operation + right.Type())
}

func evalStringInfix(left *String, right *String, operation string) (Object, error) {
	switch operation {
	case lexer.ADD:
		return &String{Value: left.Value + right.Value}, nil
	}
	return &Error{}, errors.New("BinaryOperationError: Unsupported Operation Between Strings " + operation)
}

func evalNumberInfix(left Number, right Number, operation string) (Object, error) {
	leftVal := left.Inspect()
	rightVal := right.Inspect()

	if left.Type() == lexer.UNIT_OBJ && right.Type() == lexer.UNIT_OBJ {
		convertedLeft, err := convert(left.(*Unit).Inspect(), left.(*Unit).Unit, right.(*Unit).Unit)
		if err != nil {
			return &Error{}, err
		}

		leftVal = convertedLeft.Inspect()
	}

	value := formatFloat(binaryOperations(leftVal, rightVal, operation))

	if left.Type() == lexer.UNIT_OBJ && right.Type() == lexer.UNIT_OBJ {
		return &Unit{Value: value.Inspect(), Unit: right.(*Unit).Unit}, nil

	} else if left.Type() == lexer.UNIT_OBJ {
		return &Unit{Value: value.Inspect(), Unit: left.(*Unit).Unit}, nil

	} else if right.Type() == lexer.UNIT_OBJ {
		return &Unit{Value: value.Inspect(), Unit: right.(*Unit).Unit}, nil
	}

	return value, nil
}

func binaryOperations(left float64, right float64, operation string) float64 {
	var result float64
	switch operation {
	case lexer.ADD:
		result = left + right
	case lexer.SUB:
		result = left - right
	case lexer.DIV:
		result = left / right
	case lexer.MUL:
		result = left * right
	case lexer.POW:
		result = math.Pow(left, right)
	case lexer.MOD:
		result = math.Mod(left, right)
	case lexer.EE:
		result = float64(boolToInt(left == right))
	case lexer.NE:
		result = float64(boolToInt(left != right))
	case lexer.LT:
		result = float64(boolToInt(left < right))
	case lexer.LTE:
		result = float64(boolToInt(left <= right))
	case lexer.GT:
		result = float64(boolToInt(left > right))
	case lexer.GTE:
		result = float64(boolToInt(left >= right))
	}
	return result
}

/* ---------------------------- Helper Functions ---------------------------- */

func boolToInt(value bool) int {
	if value {
		return 1
	} else {
		return 0
	}
}

func isNumberObject(object interface{}) (Number, error) {
	if obj, ok := object.(Number); ok {
		return obj, nil
	}
	return &Integer{0}, errors.New("EvaluatorError:" + object.(Object).String() + " is not type Number")
}

func formatFloat(float float64) Number {
	if float64(int(float)) == float {
		return &Integer{Value: int(float)}
	}
	// Format Float to 5 dp
	return &Float{Value: math.Round(float*100000) / 100000}
}

func handleReturn(obj Object, err error) (Object, error) {
	if err != nil {
		return &Error{}, err
	}
	return obj, nil
}

func convert(u float64, from string, to string) (*Unit, error) {
	if from == to {
		return &Unit{Value: u, Unit: from}, nil
	}

	leftUnit, err := units.Find(from)
	if err != nil {
		return &Unit{}, errors.New("ConversionError: Unit " + from + " not defined")
	}

	rightUnit, err := units.Find(to)
	if err != nil {
		return &Unit{}, errors.New("ConversionError: Unit " + to + " not defined")
	}

	return &Unit{formatFloat(units.MustConvertFloat(u, leftUnit, rightUnit).Float()).Inspect(), to}, nil
}

func lookupElements(elementIdentifier string, periodicTable map[string]interface{}) (map[string]interface{}, error) {
	for _, element := range periodicTable["elements"].([]interface{}) {
		if element.(map[string]interface{})["symbol"].(string) == elementIdentifier {
			return element.(map[string]interface{}), nil

		} else if strings.EqualFold(element.(map[string]interface{})["name"].(string), elementIdentifier) {
			return element.(map[string]interface{}), nil
		}
	}

	return map[string]interface{}{}, errors.New("EvaluationError: Identifier undefined")
}

func similarity(a, b string) float64 {
	sequencer := strutil.Similarity(a, b, metrics.NewLevenshtein())

	i := 0
	for i < len(a) && i < len(b) {
		if a[i] != b[i] {
			break
		}
		i++
	}

	consecutiveCertainty := i / len(a)
	return (float64(consecutiveCertainty) * 0.5) + (sequencer * 0.5)
}
