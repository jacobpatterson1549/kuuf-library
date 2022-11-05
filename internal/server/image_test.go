package server

import (
	"image"
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
