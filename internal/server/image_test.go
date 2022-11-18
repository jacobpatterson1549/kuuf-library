package server

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"testing"
)

func TestScaleRect(t *testing.T) {
	tests := []struct {
		name                                       string
		srcW, srcH, boundsW, boundsH, wantW, wantH int
	}{
		{"no change", 4, 3, 4, 3, 4, 3},
		{"large square-square", 470, 470, 256, 256, 256, 256},
		{"small square-square", 100, 100, 256, 256, 256, 256},
		{"too large, wide", 1920, 1200, 256, 256, 256, 160},
		{"too large, tall", 428, 721, 256, 256, 151, 256},
		{"too small, wide", 100, 63, 256, 256, 256, 161},
		{"too small, tall", 79, 100, 256, 256, 202, 256},
		{"ultra tall, skinny", 16, 512, 256, 256, 8, 256},
		{"ultra wide, short", 1024, 8, 256, 256, 256, 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srcR := image.Rect(0, 0, test.srcW, test.srcH)
			boundsR := image.Rect(0, 0, test.boundsW, test.boundsH)
			wantR := image.Rect(0, 0, test.wantW, test.wantH)
			gotR := scaleRect(srcR, boundsR)
			if wantR != gotR {
				t.Errorf("not equal: \n wanted: %v \n got:    %v", wantR, gotR)
			}
		})
	}
}

const webp1pxHex = `524946463600000057454250565038202a0000007001009d012a0100010002003425a0027401400000fef1dc8ffd958fffd077fffa0eff6ab832db4f8000`

func TestReadImage(t *testing.T) {
	onePxRect := image.Rect(0, 0, 1, 1)
	tests := []struct {
		name        string
		contentType string
		genImage    func() io.Reader
		wantOk      bool
	}{
		{
			name:        "jpg",
			contentType: "image/jpeg",
			genImage: func() io.Reader {
				var buf bytes.Buffer
				img := image.NewGray(onePxRect)
				jpeg.Encode(&buf, img, nil)
				return &buf
			},
			wantOk: true,
		},
		{
			name:        "png",
			contentType: "image/png",
			genImage: func() io.Reader {
				var buf bytes.Buffer
				img := image.NewGray(onePxRect)
				png.Encode(&buf, img)
				return &buf
			},
			wantOk: true,
		},
		{
			name:        "jpg passed as png",
			contentType: "image/png",
			genImage: func() io.Reader {
				var buf bytes.Buffer
				img := image.NewGray(onePxRect)
				jpeg.Encode(&buf, img, nil)
				return &buf
			},
		},
		{
			name:        "webp",
			contentType: "image/webp",
			genImage: func() io.Reader {
				b, _ := hex.DecodeString(webp1pxHex)
				return bytes.NewReader(b)
			},
			wantOk: true,
		},
		{
			name:        "pbm",
			contentType: "image/pbm",
			genImage: func() io.Reader {
				b := []byte("P1 \n 1 1 \n 0")
				return bytes.NewReader(b)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := test.genImage()
			_, err := readImage(r, test.contentType)
			switch {
			case !test.wantOk:
				if err == nil {
					t.Errorf("wanted error")
				}
			case err != nil:
				t.Errorf("unwanted error: %v", err)
			}
		})
	}
}

func TestImageNeedsUpdating(t *testing.T) {
	b, err := hex.DecodeString(webp1pxHex)
	if err != nil {
		t.Errorf("could not decode 1px webp image")
	}
	webp1pxBase64 := base64.StdEncoding.EncodeToString(b)
	tests := []struct {
		name        string
		imageBase64 string
		want        bool
	}{
		{"empty", "", false},
		{"invalid base64", "INVALID", true},
		{"invalid webp", "deadbeef", true},
		{"small image", webp1pxBase64, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.want != imageNeedsUpdating(test.imageBase64) {
				t.Error()
			}
		})
	}
}
