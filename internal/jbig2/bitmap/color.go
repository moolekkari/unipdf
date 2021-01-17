package bitmap

// Color is the jbig2 color interpretation enum.
// The naming convention taken from 'https://en.wikipedia.org/wiki/Binary_image#Interpretation'.
type Color int

const (
	// Vanilla is the bit interpretation where the 1'th bit means white and the 0'th bit means black.
	Vanilla Color = iota
	// Chocolate is the bit interpretation where the 0'th bit means white and the 1'th bit means black.
	Chocolate
)
