package render

import (
	"fmt"
	"image"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// ImageToHalfBlocks converts img into Unicode half-block characters (▀)
// with 24-bit ANSI fg (top pixel) and bg (bottom pixel) colors.
// termWidth is columns; pixelHeight is pixel rows (will be rounded up to even).
func ImageToHalfBlocks(img image.Image, termWidth, pixelHeight int) string {
	if img == nil || termWidth <= 0 || pixelHeight <= 0 {
		return ""
	}
	if pixelHeight%2 != 0 {
		pixelHeight++
	}

	dst := image.NewRGBA(image.Rect(0, 0, termWidth, pixelHeight))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)

	var sb strings.Builder
	sb.Grow(termWidth * pixelHeight * 20)

	for y := 0; y < pixelHeight-1; y += 2 {
		for x := 0; x < termWidth; x++ {
			tr, tg, tb, _ := dst.At(x, y).RGBA()
			br, bg, bb, _ := dst.At(x, y+1).RGBA()
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀",
				tr>>8, tg>>8, tb>>8,
				br>>8, bg>>8, bb>>8)
		}
		sb.WriteString("\x1b[0m\n")
	}
	return sb.String()
}
