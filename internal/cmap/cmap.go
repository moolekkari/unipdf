package cmap

import (
	"fmt"
	"sort"
	"strings"

	"maze.io/x/unipdf/common"
	"maze.io/x/unipdf/core"
	"maze.io/x/unipdf/internal/cmap/bcmaps"
)

const (
	// Maximum number of possible bytes per code.
	maxCodeLen = 4

	// MissingCodeRune replaces runes that can't be decoded. '\ufffd' = �. Was '?'.
	MissingCodeRune = '\ufffd' // �
)

// CharCode is a character code or Unicode
// rune is int32 https://golang.org/doc/go1#rune
type CharCode uint32

// Codespace represents a single codespace range used in the CMap.
type Codespace struct {
	NumBytes int
	Low      CharCode
	High     CharCode
}

type charRange struct {
	code0 CharCode
	code1 CharCode
}
type fbRange struct {
	code0 CharCode
	code1 CharCode
	r0    rune
}

// CIDSystemInfo contains information for identifying the character collection
// used by a CID font.
// CIDSystemInfo=Dict("Registry": Adobe, "Ordering": Korea1, "Supplement": 0, )
type CIDSystemInfo struct {
	Registry   string
	Ordering   string
	Supplement int
}

// NewCIDSystemInfo returns the CIDSystemInfo encoded in PDFObject `obj`.
func NewCIDSystemInfo(obj core.PdfObject) (info CIDSystemInfo, err error) {
	d, ok := core.GetDict(obj)
	if !ok {
		return CIDSystemInfo{}, core.ErrTypeError
	}
	registry, ok := core.GetStringVal(d.Get("Registry"))
	if !ok {
		return CIDSystemInfo{}, core.ErrTypeError
	}
	ordering, ok := core.GetStringVal(d.Get("Ordering"))
	if !ok {
		return CIDSystemInfo{}, core.ErrTypeError
	}
	supplement, ok := core.GetIntVal(d.Get("Supplement"))
	if !ok {
		return CIDSystemInfo{}, core.ErrTypeError
	}
	return CIDSystemInfo{
		Registry:   registry,
		Ordering:   ordering,
		Supplement: supplement,
	}, nil
}

// String returns a human readable description of `info`.
// It looks like "Adobe-Japan2-000".
func (info *CIDSystemInfo) String() string {
	return fmt.Sprintf("%s-%s-%03d", info.Registry, info.Ordering, info.Supplement)
}

// CMap represents a character code to unicode mapping used in PDF files.
// References:
//  https://www.adobe.com/content/dam/acom/en/devnet/acrobat/pdfs/5411.ToUnicode.pdf
//  https://github.com/adobe-type-tools/cmap-resources/releases
type CMap struct {
	*cMapParser

	name       string
	nbits      int // 8 bits for simple fonts, 16 bits for CID fonts.
	ctype      int
	version    string
	usecmap    string // Base this cmap on `usecmap` if `usecmap` is not empty.
	systemInfo CIDSystemInfo

	// For regular cmaps.
	codespaces []Codespace

	// Used by ctype 1 CMaps.
	codeToCID map[CharCode]CharCode // charcode -> CID
	cidToCode map[CharCode]CharCode // CID -> charcode

	// Used by ctype 2 CMaps.
	codeToUnicode map[CharCode]rune // CID -> Unicode
	unicodeToCode map[rune]CharCode // Unicode -> CID
}

// NewToUnicodeCMap returns an identity CMap with codeToUnicode matching the `codeToUnicode` arg.
func NewToUnicodeCMap(codeToUnicode map[CharCode]rune) *CMap {
	cmap := &CMap{
		name:  "Adobe-Identity-UCS",
		ctype: 2,
		nbits: 16,
		systemInfo: CIDSystemInfo{
			Registry:   "Adobe",
			Ordering:   "UCS",
			Supplement: 0,
		},
		codespaces:    []Codespace{{Low: 0, High: 0xffff}},
		codeToCID:     make(map[CharCode]CharCode),
		cidToCode:     make(map[CharCode]CharCode),
		codeToUnicode: codeToUnicode,
		unicodeToCode: make(map[rune]CharCode),
	}

	cmap.computeInverseMappings()
	return cmap
}

// newCMap returns an initialized CMap.
func newCMap(isSimple bool) *CMap {
	nbits := 16
	if isSimple {
		nbits = 8
	}
	return &CMap{
		nbits:         nbits,
		codeToCID:     make(map[CharCode]CharCode),
		cidToCode:     make(map[CharCode]CharCode),
		codeToUnicode: make(map[CharCode]rune),
		unicodeToCode: make(map[rune]CharCode),
	}
}

// LoadCmapFromDataCID parses the in-memory cmap `data` and returns the resulting CMap.
// It is a convenience function.
func LoadCmapFromDataCID(data []byte) (*CMap, error) {
	return LoadCmapFromData(data, false)
}

// LoadCmapFromData parses the in-memory cmap `data` and returns the resulting CMap.
// If `isSimple` is true, it uses 1-byte encodings, otherwise it uses the codespaces in the cmap.
//
// 9.10.3 ToUnicode CMaps (page 293).
func LoadCmapFromData(data []byte, isSimple bool) (*CMap, error) {
	common.Log.Trace("LoadCmapFromData: isSimple=%t", isSimple)

	cmap := newCMap(isSimple)
	cmap.cMapParser = newCMapParser(data)

	// In debugging it may help to see the data being parsed.
	// fmt.Println("===============*******===========")
	// fmt.Printf("%s\n", string(data))
	// fmt.Println("===============&&&&&&&===========")

	err := cmap.parse()
	if err != nil {
		return nil, err
	}
	if len(cmap.codespaces) == 0 {
		if cmap.usecmap != "" {
			return cmap, nil
		}

		common.Log.Debug("ERROR: No codespaces. cmap=%s", cmap)
		return nil, ErrBadCMap
	}

	cmap.computeInverseMappings()
	return cmap, nil
}

// IsPredefinedCMap returns true if the specified CMap name is a predefined
// CJK CMap. The predefined CMaps are bundled with the package and can be loaded
// using the LoadPredefinedCMap function.
// See section 9.7.5.2 "Predefined CMaps" (page 273, Table 118).
func IsPredefinedCMap(name string) bool {
	return bcmaps.AssetExists(name)
}

// LoadPredefinedCMap loads a predefined CJK CMap by name.
// See section 9.7.5.2 "Predefined CMaps" (page 273, Table 118).
func LoadPredefinedCMap(name string) (*CMap, error) {
	// Load cmap.
	cmap, err := loadPredefinedCMap(name)
	if err != nil {
		return nil, err
	}
	if cmap.usecmap == "" {
		cmap.computeInverseMappings()
		return cmap, nil
	}

	// Load base cmap.
	base, err := loadPredefinedCMap(cmap.usecmap)
	if err != nil {
		return nil, err
	}

	// Add CID ranges.
	for charcode, cid := range base.codeToCID {
		if _, ok := cmap.codeToCID[charcode]; !ok {
			cmap.codeToCID[charcode] = cid
		}
	}

	// Add codespaces.
	for _, codespace := range base.codespaces {
		cmap.codespaces = append(cmap.codespaces, codespace)
	}

	cmap.computeInverseMappings()
	return cmap, nil
}

// loadPredefinedCMap loads an embedded CMap from the bcmaps package, specified
// by name.
func loadPredefinedCMap(name string) (*CMap, error) {
	cmapData, err := bcmaps.Asset(name)
	if err != nil {
		return nil, err
	}

	return LoadCmapFromDataCID(cmapData)
}

func (cmap *CMap) computeInverseMappings() {
	// Generate CID -> charcode map.
	for code, cid := range cmap.codeToCID {
		if c, ok := cmap.cidToCode[cid]; !ok || (ok && c > code) {
			cmap.cidToCode[cid] = code
		}
	}

	// Generate Unicode -> CID map.
	for cid, r := range cmap.codeToUnicode {
		if c, ok := cmap.unicodeToCode[r]; !ok || (ok && c > cid) {
			cmap.unicodeToCode[r] = cid
		}
	}

	// Sort codespaces in order for shorter codes to be checked first.
	sort.Slice(cmap.codespaces, func(i, j int) bool {
		return cmap.codespaces[i].Low < cmap.codespaces[j].Low
	})
}

// CharcodeBytesToUnicode converts a byte array of charcodes to a unicode string representation.
// It also returns a bool flag to tell if the conversion was successful.
// NOTE: This only works for ToUnicode cmaps.
func (cmap *CMap) CharcodeBytesToUnicode(data []byte) (string, int) {
	charcodes, matched := cmap.BytesToCharcodes(data)
	if !matched {
		common.Log.Debug("ERROR: CharcodeBytesToUnicode. Not in codespaces. data=[% 02x] cmap=%s",
			data, cmap)
		return "", 0
	}

	var (
		parts   []rune
		missing []CharCode
	)
	for _, code := range charcodes {
		s, ok := cmap.codeToUnicode[code]
		if !ok {
			missing = append(missing, code)
			s = MissingCodeRune
		}
		parts = append(parts, s)
	}
	unicode := string(parts)
	if len(missing) > 0 {
		common.Log.Debug("ERROR: CharcodeBytesToUnicode. Not in map.\n"+
			"\tdata=[% 02x]=%#q\n"+
			"\tcharcodes=%02x\n"+
			"\tmissing=%d %02x\n"+
			"\tunicode=`%s`\n"+
			"\tcmap=%s",
			data, string(data), charcodes, len(missing), missing, unicode, cmap)
	}
	return unicode, len(missing)
}

// CharcodeToUnicode converts a single character code `code` to a unicode string.
// If `code` is not in the unicode map, '�' is returned.
// NOTE: CharcodeBytesToUnicode is typically more efficient.
func (cmap *CMap) CharcodeToUnicode(code CharCode) (rune, bool) {
	if s, ok := cmap.codeToUnicode[code]; ok {
		return s, true
	}
	return MissingCodeRune, false
}

// RuneToCID maps the specified rune to a character identifier. If the provided
// rune has no available mapping, the second return value is false.
func (cmap *CMap) RuneToCID(r rune) (CharCode, bool) {
	cid, ok := cmap.unicodeToCode[r]
	return cid, ok
}

// CharcodeToCID maps the specified character code to a character identifier.
// If the provided charcode has no available mapping, the second return value
// is false. The returned CID can be mapped to a Unicode character using a
// Unicode conversion CMap.
func (cmap *CMap) CharcodeToCID(code CharCode) (CharCode, bool) {
	cid, ok := cmap.codeToCID[code]
	return cid, ok
}

// CIDToCharcode maps the specified character identified to a character code. If
// the provided CID has no available mapping, the second return value is false.
func (cmap *CMap) CIDToCharcode(cid CharCode) (CharCode, bool) {
	code, ok := cmap.cidToCode[cid]
	return code, ok
}

// BytesToCharcodes attempts to convert the entire byte array `data` to a list
// of character codes from the ranges specified by `cmap`'s codespaces.
// Returns:
//      character code sequence (if there is a match complete match)
//      matched?
// NOTE: A partial list of character codes will be returned if a complete match
//       is not possible.
func (cmap *CMap) BytesToCharcodes(data []byte) ([]CharCode, bool) {
	var charcodes []CharCode
	if cmap.nbits == 8 {
		for _, b := range data {
			charcodes = append(charcodes, CharCode(b))
		}
		return charcodes, true
	}
	for i := 0; i < len(data); {
		code, n, matched := cmap.matchCode(data[i:])
		if !matched {
			common.Log.Debug("ERROR: No code match at i=%d bytes=[% 02x]=%#q", i, data, string(data))
			return charcodes, false
		}
		charcodes = append(charcodes, code)
		i += n
	}
	return charcodes, true
}

// Name returns the name of the CMap.
func (cmap *CMap) Name() string {
	return cmap.name
}

// Type returns the CMap type.
func (cmap *CMap) Type() int {
	return cmap.ctype
}

// Nbits returns 8 bits for simple font CMaps and 16 bits for CID font CMaps.
func (cmap *CMap) NBits() int {
	return cmap.nbits
}

// String returns a human readable description of `cmap`.
func (cmap *CMap) String() string {
	si := cmap.systemInfo
	parts := []string{
		fmt.Sprintf("nbits:%d", cmap.nbits),
		fmt.Sprintf("type:%d", cmap.ctype),
	}
	if cmap.version != "" {
		parts = append(parts, fmt.Sprintf("version:%s", cmap.version))
	}
	if cmap.usecmap != "" {
		parts = append(parts, fmt.Sprintf("usecmap:%#q", cmap.usecmap))
	}
	parts = append(parts, fmt.Sprintf("systemInfo:%s", si.String()))
	if len(cmap.codespaces) > 0 {
		parts = append(parts, fmt.Sprintf("codespaces:%d", len(cmap.codespaces)))
	}
	if len(cmap.codeToUnicode) > 0 {
		parts = append(parts, fmt.Sprintf("codeToUnicode:%d", len(cmap.codeToUnicode)))
	}
	return fmt.Sprintf("CMAP{%#q %s}", cmap.name, strings.Join(parts, " "))
}

// Bytes returns the raw bytes of a PDF CMap corresponding to `cmap`.
func (cmap *CMap) Bytes() []byte {
	common.Log.Trace("cmap.Bytes: cmap=%s", cmap.String())
	body := cmap.toBfData()
	whole := strings.Join([]string{cmapHeader, body, cmapTrailer}, "\n")
	return []byte(whole)
}

// matchCode attempts to match the byte array `data` with a character code in `cmap`'s codespaces.
// Returns:
//      character code (if there is a match) of
//      number of bytes read (if there is a match)
//      matched?
func (cmap *CMap) matchCode(data []byte) (code CharCode, n int, matched bool) {
	for j := 0; j < maxCodeLen; j++ {
		if j < len(data) {
			code = code<<8 | CharCode(data[j])
			n++
		}
		matched = cmap.inCodespace(code, j+1)
		if matched {
			return code, n, true
		}
	}
	// No codespace matched data. This is a serious problem.
	common.Log.Debug("ERROR: No codespace matches bytes=[% 02x]=%#q cmap=%s",
		data, string(data), cmap)
	return 0, 0, false
}

// inCodespace returns true if `code` is in the `numBytes` byte codespace.
func (cmap *CMap) inCodespace(code CharCode, numBytes int) bool {
	for _, cs := range cmap.codespaces {
		if cs.Low <= code && code <= cs.High && numBytes == cs.NumBytes {
			return true
		}
	}
	return false
}

// toBfData returns the bfchar and bfrange sections of a CMap text file.
// Both sections are computed from cmap.codeToUnicode.
func (cmap *CMap) toBfData() string {
	if len(cmap.codeToUnicode) == 0 {
		return ""
	}

	// codes is a sorted list of the codeToUnicode keys.
	var codes []CharCode
	for code := range cmap.codeToUnicode {
		codes = append(codes, code)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })

	// charRanges is a list of the contiguous character code ranges in `codes`.
	var charRanges []charRange
	c0, c1 := codes[0], codes[0]+1
	for _, c := range codes[1:] {
		if c != c1 {
			charRanges = append(charRanges, charRange{c0, c1})
			c0 = c
		}
		c1 = c + 1
	}
	if c1 > c0 {
		charRanges = append(charRanges, charRange{c0, c1})
	}

	// fbChars is a list of single character ranges. fbRanges is a list of multiple character ranges.
	var fbChars []CharCode
	var fbRanges []fbRange
	for _, cr := range charRanges {
		if cr.code0+1 == cr.code1 {
			fbChars = append(fbChars, cr.code0)
		} else {
			fbRanges = append(fbRanges, fbRange{
				code0: cr.code0,
				code1: cr.code1,
				r0:    cmap.codeToUnicode[cr.code0],
			})
		}
	}
	common.Log.Trace("charRanges=%d fbChars=%d fbRanges=%d", len(charRanges), len(fbChars),
		len(fbRanges))

	var lines []string
	if len(fbChars) > 0 {
		numRanges := (len(fbChars) + maxBfEntries - 1) / maxBfEntries
		for i := 0; i < numRanges; i++ {
			n := min(len(fbChars)-i*maxBfEntries, maxBfEntries)
			lines = append(lines, fmt.Sprintf("%d beginbfchar", n))
			for j := 0; j < n; j++ {
				code := fbChars[i*maxBfEntries+j]
				r := cmap.codeToUnicode[code]
				lines = append(lines, fmt.Sprintf("<%04x> <%04x>", code, r))
			}
			lines = append(lines, "endbfchar")
		}
	}
	if len(fbRanges) > 0 {
		numRanges := (len(fbRanges) + maxBfEntries - 1) / maxBfEntries
		for i := 0; i < numRanges; i++ {
			n := min(len(fbRanges)-i*maxBfEntries, maxBfEntries)
			lines = append(lines, fmt.Sprintf("%d beginbfrange", n))
			for j := 0; j < n; j++ {
				rng := fbRanges[i*maxBfEntries+j]
				r := rng.r0
				lines = append(lines, fmt.Sprintf("<%04x><%04x> <%04x>", rng.code0, rng.code1-1, r))
			}
			lines = append(lines, "endbfrange")
		}
	}
	return strings.Join(lines, "\n")
}

const (
	maxBfEntries = 100 // Maximum number of entries in a bfchar or bfrange section.
	cmapHeader   = `
/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
`
	cmapTrailer = `endcmap
CMapName currentdict /CMap defineresource pop
end
end
`
)
