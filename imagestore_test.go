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
	"github.com/writefreely/writefreely/config"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// tinyPNG returns the bytes of a valid 2x2 PNG.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	assert.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// otherTinyPNG returns a valid 2x2 PNG whose bytes differ from tinyPNG's, for
// the cases that need two distinct images rather than two copies of one.
func otherTinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{0, 0, 255, 255})
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

func TestPrepareUploadRejectsHTMLNamedAsPNG(t *testing.T) {
	_, _, _, err := prepareUpload([]byte("<html><script>alert(1)</script></html>"))
	assert.ErrorIs(t, err, errUnsupportedImageType,
		"content, not the filename, decides the type")
}

func TestPrepareUploadRejectsWebP(t *testing.T) {
	// A minimal RIFF/WEBP header is enough: we must refuse it rather than
	// pulling in golang.org/x/image to decode it.
	webp := append([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), make([]byte, 32)...)
	_, _, _, err := prepareUpload(webp)
	assert.ErrorIs(t, err, errUnsupportedImageType)
}

func TestPrepareUploadAcceptsPNG(t *testing.T) {
	out, mime, ext, err := prepareUpload(tinyPNG(t))
	assert.NoError(t, err)
	assert.Equal(t, "image/png", mime)
	assert.Equal(t, "png", ext)
	assert.Equal(t, "image/png", sniffImageType(out), "output must still be a PNG")
}

func TestPrepareUploadAcceptsJPEG(t *testing.T) {
	out, mime, ext, err := prepareUpload(tinyJPEG(t))
	assert.NoError(t, err)
	assert.Equal(t, "image/jpeg", mime)
	assert.Equal(t, "jpg", ext)
	assert.Equal(t, "image/jpeg", sniffImageType(out))
}

func TestPrepareUploadKeepsGIFAnimation(t *testing.T) {
	out, mime, ext, err := prepareUpload(animatedGIF(t))
	assert.NoError(t, err)
	assert.Equal(t, "image/gif", mime)
	assert.Equal(t, "gif", ext)

	g, err := gif.DecodeAll(bytes.NewReader(out))
	assert.NoError(t, err)
	assert.Len(t, g.Image, 2, "every frame of an animated GIF must survive re-encoding")
}

func TestPrepareUploadAcceptsOnlyBuiltInTypes(t *testing.T) {
	// The accepted set is fixed by the decoders compiled in, not by
	// configuration, so that no deployment can widen it into a format the
	// re-encode step cannot sanitize.
	for _, c := range []struct {
		name string
		in   []byte
		mime string
	}{
		{"png", tinyPNG(t), "image/png"},
		{"jpeg", tinyJPEG(t), "image/jpeg"},
		{"gif", animatedGIF(t), "image/gif"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, mime, _, err := prepareUpload(c.in)
			assert.NoError(t, err)
			assert.Equal(t, c.mime, mime)
		})
	}
}

func TestPrepareUploadRejectsTruncatedImage(t *testing.T) {
	full := tinyPNG(t)
	_, _, _, err := prepareUpload(full[:len(full)/2])
	assert.ErrorIs(t, err, errUnsupportedImageType,
		"bytes that sniff as an image but will not decode are not stored")
}

func TestReencodeStripsMetadata(t *testing.T) {
	// A JPEG carrying an APP1/Exif segment must come out without it.
	withExif := jpegWithExifSegment(t)
	assert.True(t, bytes.Contains(withExif, []byte("Exif")), "fixture must actually contain Exif")
	assert.Equal(t, "image/jpeg", sniffImageType(withExif), "fixture must still be a JPEG")

	out, _, _, err := prepareUpload(withExif)
	assert.NoError(t, err)
	assert.False(t, bytes.Contains(out, []byte("Exif")),
		"re-encoding must discard camera metadata, which routinely includes GPS coordinates")
	assert.False(t, bytes.Contains(out, []byte("SECRETGPS")))
}

func TestImagePathNamesTheDayAndTheFile(t *testing.T) {
	at := time.Date(2026, 3, 9, 15, 4, 5, 0, time.UTC)
	assert.Equal(t, "2026/03/09/holiday-snap.png", imagePath("Holiday Snap.PNG", "png", at, 1))
	assert.Equal(t, "2026/03/09/holiday-snap-2.png", imagePath("Holiday Snap.PNG", "png", at, 2),
		"a name already used that day is disambiguated rather than overwritten")
}

func TestImagePathIsAlwaysUnderItsDirectory(t *testing.T) {
	at := time.Date(2026, 3, 9, 15, 4, 5, 0, time.UTC)
	for _, name := range []string{
		"../../etc/passwd.png",
		"..\\..\\windows\\system32.png",
		"/absolute/path.png",
		"....//....//escape.png",
		"nul.png",
	} {
		p := imagePath(name, "png", at, 1)
		assert.True(t, strings.HasPrefix(p, "2026/03/09/"), name)
		assert.Equal(t, 3, strings.Count(p, "/"), "the name cannot add directories: "+p)
		assert.False(t, strings.Contains(p, ".."), name)
	}
}

func TestImageSlugFallsBackWhenNothingSurvives(t *testing.T) {
	assert.Equal(t, "image", imageSlug(".png"), "a file with no name still needs one")
	assert.Equal(t, "image", imageSlug("???.png"))
	assert.Equal(t, "image", imageSlug(""))
}

func TestImageSlugIsBounded(t *testing.T) {
	s := imageSlug(strings.Repeat("long-name-", 40) + ".png")
	assert.LessOrEqual(t, len(s), imageSlugMaxLen)
	assert.False(t, strings.HasSuffix(s, "-"), "a truncated slug must not end mid-separator")
}

func TestImageSlugTransliterates(t *testing.T) {
	assert.Equal(t, "uber-cafe.png", imagePath("Über Café.png", "png",
		time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), 1)[len("2026/03/09/"):])
}

func TestSha256Hex(t *testing.T) {
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", sha256Hex([]byte("hello")))
}

func TestPrepareUploadAcceptsSVG(t *testing.T) {
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1"/></svg>`)
	out, mime, ext, err := prepareUpload(svg)
	assert.NoError(t, err)
	assert.Equal(t, svgMIME, mime)
	assert.Equal(t, "svg", ext)
	// SVG is a document, not a bitmap, so it is stored exactly as uploaded
	// rather than decoded and re-encoded.
	assert.Equal(t, svg, out)
}

func TestLooksLikeSVG(t *testing.T) {
	yes := [][]byte{
		[]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		[]byte(`  <SVG xmlns="http://www.w3.org/2000/svg"></SVG>`),
		[]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`),
		[]byte(`<!-- a comment --><svg/>`),
		[]byte("\xef\xbb\xbf<svg/>"),
	}
	for _, b := range yes {
		assert.True(t, looksLikeSVG(b), "%q", string(b))
	}

	no := [][]byte{
		[]byte(`<html><script>alert(1)</script></html>`),
		[]byte(`<?xml version="1.0"?><rss><channel/></rss>`),
		[]byte("plain text"),
		{},
	}
	for _, b := range no {
		assert.False(t, looksLikeSVG(b), "%q", string(b))
	}
}

// TestPrepareUploadStillRejectsNonSVGXML guards the SVG path from becoming
// a way to store arbitrary XML.
func TestPrepareUploadStillRejectsNonSVGXML(t *testing.T) {
	_, _, _, err := prepareUpload([]byte(`<?xml version="1.0"?><rss><channel/></rss>`))
	assert.ErrorIs(t, err, errUnsupportedImageType)
}

// TestExtForMIMEMatchesPrepareUpload guards the mapping that the stored
// path and the public URL both depend on. They are built independently, so
// a format accepted by prepareUpload but unknown here was written to disk
// under one name and served under another, giving a 404.
func TestExtForMIMEMatchesPrepareUpload(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"png", tinyPNG(t)},
		{"jpeg", tinyJPEG(t)},
		{"gif", animatedGIF(t)},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, mime, ext, err := prepareUpload(c.in)
			assert.NoError(t, err)
			assert.Equal(t, ext, extForMIME(mime),
				"the URL's extension must match the one the file is stored under")
			assert.Equal(t, c.name == "jpeg" && ext == "jpg" || ext == c.name, true,
				"unexpected extension %q for %s", ext, c.name)
		})
	}
}

func TestEnsureUploadsWritable(t *testing.T) {
	dir := t.TempDir()
	cfg := config.New()
	cfg.Server.StaticParentDir = dir
	app := &App{cfg: cfg}

	// Creates the directory when it is missing, rather than requiring the
	// deployment to have made it.
	assert.NoError(t, app.ensureUploadsWritable())
	info, err := os.Stat(app.uploadsRoot())
	assert.NoError(t, err)
	assert.True(t, info.IsDir())

	// Leaves nothing behind.
	entries, err := os.ReadDir(app.uploadsRoot())
	assert.NoError(t, err)
	assert.Empty(t, entries)

	// Reports a directory it cannot write to, which is how a container
	// image with a root-owned uploads directory presents.
	if os.Geteuid() == 0 {
		t.Skip("running as root; permissions are not enforced")
	}
	assert.NoError(t, os.Chmod(app.uploadsRoot(), 0500))
	t.Cleanup(func() { os.Chmod(app.uploadsRoot(), 0755) })
	assert.Error(t, app.ensureUploadsWritable())
}

func TestUploadsRootHonoursConfiguredDir(t *testing.T) {
	cfg := config.New()
	cfg.Server.StaticParentDir = "/srv/wf"

	// Unset, uploads live under the static tree, which keeps a default
	// install self-contained.
	app := &App{cfg: cfg}
	assert.Equal(t, filepath.Join("/srv/wf", staticDir, uploadsDir), app.uploadsRoot())

	// Set, they go exactly where the operator asked, which is how a
	// packaged install keeps user content out of the read-only asset tree.
	cfg.Uploads.Dir = "/var/lib/writefreely/uploads"
	assert.Equal(t, "/var/lib/writefreely/uploads", app.uploadsRoot())

	cfg.Uploads.Dir = "  /var/lib/writefreely/uploads  "
	assert.Equal(t, "/var/lib/writefreely/uploads", app.uploadsRoot(),
		"a value with stray whitespace should not create a directory named with it")
}
