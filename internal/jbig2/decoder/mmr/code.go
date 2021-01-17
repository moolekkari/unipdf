package mmr

import (
	"fmt"
)

type code struct {
	bitLength      int
	codeWord       int
	runLength      int
	subTable       []*code
	nonNilSubTable bool
}

func newCode(codeData [3]int) *code {
	return &code{
		bitLength: codeData[0],
		codeWord:  codeData[1],
		runLength: codeData[2],
	}
}

// String implements Stringer interface.
func (c *code) String() string {
	return fmt.Sprintf("%d/%d/%d", c.bitLength, c.codeWord, c.runLength)
}
