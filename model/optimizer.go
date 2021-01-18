package model

import (
	"github.com/moolekkari/unipdf/core"
)

// Optimizer is the interface that performs optimization of PDF object structure for output writing.
//
// Optimize receives a slice of input `objects`, performs optimization, including removing, replacing objects and
// output the optimized slice of objects.
type Optimizer interface {
	Optimize(objects []core.PdfObject) ([]core.PdfObject, error)
}
