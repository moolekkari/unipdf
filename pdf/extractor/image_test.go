/*
 * This file is subject to the terms and conditions defined in
 * file 'LICENSE.md', which is part of this source code package.
 */

package extractor

import (
	"math"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unidoc/unidoc/pdf/core"
	"github.com/unidoc/unidoc/pdf/model"
)

func loadPageFromPDFFile(filePath string, pageNum int) (*model.PdfPage, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pdfReader, err := model.NewPdfReader(f)
	if err != nil {
		return nil, err
	}

	return pdfReader.GetPage(pageNum)
}

func TestImageExtractionBasic(t *testing.T) {
	type expectedImage struct {
		X      float64
		Y      float64
		Width  float64
		Height float64
		Angle  int
	}

	testcases := []struct {
		Name     string
		PageNum  int
		Path     string
		Expected []ImageMark
	}{
		{
			"basic xobject",
			1,
			"./testdata/basic_xobject.pdf",
			[]ImageMark{
				{
					Image:  nil,
					X:      0,
					Y:      294.865385,
					Width:  612,
					Height: 197.134615,
					Angle:  0,
				},
			},
		},
		{
			"inline image",
			1,
			"./testdata/inline.pdf",
			[]ImageMark{
				{
					Image:  nil,
					X:      0,
					Y:      -0.000000358,
					Width:  12,
					Height: 12,
					Angle:  0,
				},
			},
		},
	}

	for _, tcase := range testcases {
		page, err := loadPageFromPDFFile(tcase.Path, tcase.PageNum)
		require.NoError(t, err)

		pageExtractor, err := New(page)
		require.NoError(t, err)

		pageImages, err := pageExtractor.ExtractPageImages()
		require.NoError(t, err)

		assert.Equal(t, len(tcase.Expected), len(pageImages.Images))

		for i, img := range pageImages.Images {
			img.Image = nil // Discard image data.
			assert.Equalf(t, tcase.Expected[i], img, "i = %d", i)
		}
	}
}

// Test position extraction with nested transform matrices.
func TestImageExtractionNestedCM(t *testing.T) {
	testcases := []struct {
		Name      string
		PageNum   int
		Path      string
		PrependCS string
		AppendCS  string
		Expected  []ImageMark
	}{
		{
			"basic xobject - translate (100,50)",
			1,
			"./testdata/basic_xobject.pdf",
			"1 0 0 1 100.0 50.0 cm q",
			"Q",
			[]ImageMark{
				{
					Image:  nil,
					X:      0 + 100.0,
					Y:      294.865385 + 50.0,
					Width:  612,
					Height: 197.134615,
					Angle:  0,
				},
			},
		},
		{
			"basic xobject - scale (1.5,2)X",
			1,
			"./testdata/basic_xobject.pdf",
			"1.5 0 0 2.0 0 0 cm q",
			"Q",
			[]ImageMark{
				{
					Image:  nil,
					X:      0,
					Y:      294.865385 * 2.0,
					Width:  612 * 1.5,
					Height: 197.134615 * 2.0,
					Angle:  0,
				},
			},
		},
		{
			"basic xobject - translate (100,50) scale (1.5,2)X",
			1,
			"./testdata/basic_xobject.pdf",
			"1.5 0 0 2.0 0 0 cm q 1 0 0 1 100.0 50.0 cm q",
			"Q Q",
			[]ImageMark{
				{
					Image:  nil,
					X:      100.0 * 1.5,
					Y:      (294.865385 + 50.0) * 2.0,
					Width:  612 * 1.5,
					Height: 197.134615 * 2.0,
					Angle:  0,
				},
			},
		},
	}

	for _, tcase := range testcases {
		page, err := loadPageFromPDFFile(tcase.Path, tcase.PageNum)
		require.NoError(t, err)

		contentstr, err := page.GetAllContentStreams()
		require.NoError(t, err)

		// Modify the contentstream to alter the position by way of nested transform matrices.
		contentstr = tcase.PrependCS + " " + contentstr + " " + tcase.AppendCS
		err = page.SetContentStreams([]string{contentstr}, core.NewFlateEncoder())
		require.NoError(t, err)

		pageExtractor, err := New(page)
		require.NoError(t, err)

		pageImages, err := pageExtractor.ExtractPageImages()
		require.NoError(t, err)

		assert.Equal(t, len(tcase.Expected), len(pageImages.Images))

		for i, img := range pageImages.Images {
			img.Image = nil // Discard image data.
			assert.Equalf(t, tcase.Expected[i], img, "i = %d", i)
		}
	}
}

// Test multiple copies of same image XObject with different scales.
func TestImageExtractionMulti(t *testing.T) {
	testcases := []struct {
		PageNum       int
		Path          string
		NumImages     int
		DimensionFunc func(i int) (dy float64, w float64, h float64)
		NumSamples    int
	}{
		{
			1,
			"./testdata/multi.pdf",
			12,
			func(i int) (dy float64, w float64, h float64) {
				w = 100 + 10*float64(i+1)
				ar := 35.432692 / 110.0
				h = w * ar

				dy = h

				return dy, w, h
			},
			416 * 134 * 3,
		},
	}

	for _, tcase := range testcases {
		page, err := loadPageFromPDFFile(tcase.Path, tcase.PageNum)
		require.NoError(t, err)

		pageExtractor, err := New(page)
		require.NoError(t, err)

		pageImages, err := pageExtractor.ExtractPageImages()
		require.NoError(t, err)

		assert.Equal(t, tcase.NumImages, len(pageImages.Images))

		for i, img := range pageImages.Images {
			dy, w, h := tcase.DimensionFunc(i)

			assert.Equalf(t, tcase.NumSamples, len(img.Image.GetSamples()), "i = %d", i)

			// Comparison with tolerance.
			assert.Truef(t, math.Abs(w-img.Width) < 0.00001, "i = %d", i)
			assert.Truef(t, math.Abs(h-img.Height) < 0.00001, "i = %d", i)

			if i > 0 {
				measDY := pageImages.Images[i-1].Y - pageImages.Images[i].Y
				assert.Truef(t, math.Abs(dy-measDY) < 0.00001, "i = %d", i)
			}
		}
	}
}

func BenchmarkImageExtraction(b *testing.B) {
	cnt := 0
	for i := 0; i < b.N; i++ {
		page, err := loadPageFromPDFFile("./testdata/basic_xobject.pdf", 1)
		require.NoError(b, err)

		pageExtractor, err := New(page)
		require.NoError(b, err)

		pageImages, err := pageExtractor.ExtractPageImages()
		require.NoError(b, err)

		cnt += len(pageImages.Images)
	}

	assert.Equal(b, b.N, cnt)
}