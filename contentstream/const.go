package contentstream

import "errors"

var (
	// ErrInvalidOperand specifies that invalid operands have been encountered
	// while parsing the content stream.
	ErrInvalidOperand = errors.New("invalid operand")
)
