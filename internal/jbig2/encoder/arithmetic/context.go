package arithmetic

type codingContext struct {
	context  []byte
	mpsTable []byte
}

func (c *codingContext) mps(num uint32) int {
	return int(c.mpsTable[num])
}

func (c *codingContext) flipMps(num uint32) {
	c.mpsTable[num] = 1 - c.mpsTable[num]
}

func newContext(size int) *codingContext {
	return &codingContext{
		context:  make([]byte, size),
		mpsTable: make([]byte, size),
	}
}
