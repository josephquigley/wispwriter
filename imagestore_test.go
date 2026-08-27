/*
 * Copyright © 2026 Musing Studio LLC.
 *
 * This file is part of WriteFreely.
 *
 * WriteFreely is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License, included
 * in the LICENSE file in this source code package.
 */

package writefreely

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var defaultAllowed = []string{"image/png", "image/jpeg", "image/gif"}

// tinyPNG returns the bytes of a valid 2x2 PNG.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	assert.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// tinyJPEG returns the bytes of a valid 2x2 JPEG.
func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	assert.NoError(t, jpeg.Encode(&buf, img, nil))
	return buf.Bytes()
}

// animatedGIF returns the bytes of a valid two-frame GIF.
func animatedGIF(t *testing.T) []byte {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	g := &gif.GIF{}
	for i := 0; i < 2; i++ {
		f := image.NewPaletted(image.Rect(0, 0, 2, 2), pal)
		f.SetColorIndex(0, 0, uint8(i))
		g.Image = append(g.Image, f)
		g.Delay = append(g.Delay, 10)
	}
	var buf bytes.Buffer
	assert.NoError(t, gif.EncodeAll(&buf, g))
	return buf.Bytes()
}

// jpegWithExifSegment returns a valid JPEG carrying an APP1 segment whose
// payload is the ASCII "Exif\x00\x00" marker, standing in for the camera
// metadata a phone photo would carry.
func jpegWithExifSegment(t *testing.T) []byte {
	t.Helper()
	base := tinyJPEG(t)
	// A JPEG starts with the two-byte SOI marker; the APP1 segment is
	// spliced in directly after it.
	payload := []byte("Exif\x00\x00SECRETGPS")
	seg := []byte{0xFF, 0xE1}
	length := make([]byte, 2)
	binary.BigEndian.PutUint16(length, uint16(len(payload)+2))
	seg = append(seg, length...)
	seg = append(seg, payload...)

	out := make([]byte, 0, len(base)+len(seg))
	out = append(out, base[:2]...)
	out = append(out, seg...)
	out = append(out, base[2:]...)
	return out
}

func TestSniffImageType(t *testing.T) {
	assert.Equal(t, "image/png", sniffImageType(tinyPNG(t)))
	assert.Equal(t, "image/jpeg", sniffImageType(tinyJPEG(t)))
	assert.NotContains(t, sniffImageType([]byte("<html><body>hi</body></html>")), "image/")
}

func TestDecodeAndReencodeRejectsHTMLNamedAsPNG(t *testing.T) {
	_, _, _, err := decodeAndReencode([]byte("<html><script>alert(1)</script></html>"), defaultAllowed)
	assert.ErrorIs(t, err, errUnsupportedImageType,
		"content, not the filename, decides the type")
}

func TestDecodeAndReencodeRejectsSVG(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	_, _, _, err := decodeAndReencode(svg, defaultAllowed)
	assert.ErrorIs(t, err, errUnsupportedImageType,
		"SVG is script-bearing and must never be stored")
}

func TestDecodeAndReencodeRejectsWebP(t *testing.T) {
	// A minimal RIFF/WEBP header is enough: we must refuse it rather than
	// pulling in golang.org/x/image to decode it.
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
	_, _, _, err := decodeAndReencode(webp, defaultAllowed)
	assert.ErrorIs(t, err, errUnsupportedImageType)
}

func TestDecodeAndReencodeAcceptsPNG(t *testing.T) {
	out, mime, ext, err := decodeAndReencode(tinyPNG(t), defaultAllowed)
	assert.NoError(t, err)
	assert.Equal(t, "image/png", mime)
	assert.Equal(t, "png", ext)
	assert.Equal(t, "image/png", sniffImageType(out), "output must still be a PNG")
}

func TestDecodeAndReencodeAcceptsJPEG(t *testing.T) {
	out, mime, ext, err := decodeAndReencode(tinyJPEG(t), defaultAllowed)
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", mime)
	assert.Equal(t, "jpg", ext)
	assert.Equal(t, "image/jpeg", sniffImageType(out))
}

func TestDecodeAndReencodeKeepsGIFAnimation(t *testing.T) {
	out, mime, ext, err := decodeAndReencode(animatedGIF(t), defaultAllowed)
	assert.NoError(t, err)
	assert.Equal(t, "image/gif", mime)
	assert.Equal(t, "gif", ext)

	g, err := gif.DecodeAll(bytes.NewReader(out))
	assert.NoError(t, err)
	assert.Len(t, g.Image, 2, "every frame of an animated GIF must survive re-encoding")
}

func TestDecodeAndReencodeHonoursAllowList(t *testing.T) {
	_, _, _, err := decodeAndReencode(tinyPNG(t), []string{"image/jpeg"})
	assert.ErrorIs(t, err, errUnsupportedImageType,
		"a type left off the allow-list must be refused even though we can decode it")
}

func TestDecodeAndReencodeRejectsTruncatedImage(t *testing.T) {
	full := tinyPNG(t)
	_, _, _, err := decodeAndReencode(full[:len(full)/2], defaultAllowed)
	assert.ErrorIs(t, err, errUnsupportedImageType,
		"bytes that sniff as an image but will not decode are not stored")
}

func TestReencodeStripsMetadata(t *testing.T) {
	// A JPEG carrying an APP1/Exif segment must come out without it.
	withExif := jpegWithExifSegment(t)
	assert.True(t, bytes.Contains(withExif, []byte("Exif")), "fixture must actually contain Exif")
	assert.Equal(t, "image/jpeg", sniffImageType(withExif), "fixture must still be a JPEG")

	out, _, _, err := decodeAndReencode(withExif, defaultAllowed)
	assert.NoError(t, err)
	assert.False(t, bytes.Contains(out, []byte("Exif")),
		"re-encoding must discard camera metadata, which routinely includes GPS coordinates")
	assert.False(t, bytes.Contains(out, []byte("SECRETGPS")))
}

func TestImagePathIsServerDerived(t *testing.T) {
	sum := sha256Hex([]byte("hello"))
	p := imagePath(42, sum, "png")
	assert.Equal(t, "42/"+sum[:2]+"/"+sum+".png", p)
	assert.False(t, strings.Contains(p, ".."), "no traversal is structurally possible")
}

func TestSha256Hex(t *testing.T) {
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", sha256Hex([]byte("hello")))
}
