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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// uploadsDir is the directory, under the static directory, that uploaded
	// images are stored in and served from.
	uploadsDir = "uploads"

	// jpegQuality is the quality uploaded JPEGs are re-encoded at.
	jpegQuality = 90
)

// errUnsupportedImageType is returned for anything we won't store: a type
// that isn't on the allow-list, and anything that sniffs as an image but
// won't actually decode.
var errUnsupportedImageType = errors.New("unsupported image type")

// sniffImageType returns the MIME type of b, determined from its content.
// The client's Content-Type header and filename are deliberately ignored:
// neither is trustworthy, and both are attacker-controlled.
func sniffImageType(b []byte) string {
	if len(b) > 512 {
		b = b[:512]
	}
	return strings.Split(http.DetectContentType(b), ";")[0]
}

// decodeAndReencode decodes an image and re-encodes it in the same format,
// discarding all metadata. Returns the new bytes, the MIME type, and the file
// extension to store it under.
//
// Re-encoding is what strips Exif, which routinely carries GPS coordinates:
// decoding to an image.Image and encoding again keeps only the pixels, so no
// Exif library is involved. The type is decided by sniffing the content
// against allowed; SVG is not decodable here and so can never be stored.
// svgMIME is the type SVG uploads are stored and served as. Go's content
// sniffer never reports it -- an SVG comes back as text/xml or text/plain
// -- so SVG is recognised separately, by looksLikeSVG.
const svgMIME = "image/svg+xml"

// looksLikeSVG reports whether b is an SVG document, allowing for a byte
// order mark, an XML declaration, comments and a doctype before the root
// element. It is deliberately conservative: anything it does not
// positively recognise falls through to the raster path and is rejected.
func looksLikeSVG(b []byte) bool {
	head := b
	if len(head) > 1024 {
		head = head[:1024]
	}
	head = bytes.TrimPrefix(head, []byte("\xef\xbb\xbf"))
	lower := bytes.ToLower(bytes.TrimSpace(head))
	if bytes.HasPrefix(lower, []byte("<svg")) {
		return true
	}
	// Skip an XML declaration, doctype or comments before the root element.
	if !bytes.HasPrefix(lower, []byte("<?xml")) && !bytes.HasPrefix(lower, []byte("<!")) {
		return false
	}
	return bytes.Contains(lower, []byte("<svg"))
}

// extForMIME returns the file extension an accepted upload is stored
// under. It is the single source of truth for that mapping: the stored
// path and the public URL are built independently, and when they disagreed
// the URL pointed at a file that was never written.
func extForMIME(mime string) string {
	switch mime {
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case svgMIME:
		return "svg"
	}
	return "png"
}

// prepareUpload validates an uploaded file and returns the bytes to store,
// its MIME type and the extension to store it under.
//
// Raster images are decoded and re-encoded, which discards every byte that
// is not pixel data -- metadata, trailing payloads, anything hostile.
// SVG cannot be treated that way: it is a document, not a bitmap, and Go
// has no rasteriser. Its bytes are therefore stored verbatim, and the
// safety of serving them is enforced at request time instead. See
// uploadHeaders.
func prepareUpload(b []byte) ([]byte, string, string, error) {
	if looksLikeSVG(b) {
		return b, svgMIME, extForMIME(svgMIME), nil
	}

	mime := sniffImageType(b)

	var buf bytes.Buffer
	switch mime {
	case "image/png":
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, "", "", errUnsupportedImageType
		}
		if err = png.Encode(&buf, img); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), mime, extForMIME(mime), nil
	case "image/jpeg":
		img, err := jpeg.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, "", "", errUnsupportedImageType
		}
		if err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), mime, extForMIME(mime), nil
	case "image/gif":
		// DecodeAll, not Decode: the latter silently reduces an animated
		// GIF to its first frame.
		g, err := gif.DecodeAll(bytes.NewReader(b))
		if err != nil {
			return nil, "", "", errUnsupportedImageType
		}
		if err = gif.EncodeAll(&buf, g); err != nil {
			return nil, "", "", err
		}
		return buf.Bytes(), mime, extForMIME(mime), nil
	}
	return nil, "", "", errUnsupportedImageType
}

// sha256Hex returns the lowercase hex SHA-256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// imagePath returns the storage path for an image, relative to the uploads
// root. Every part of it is server-derived -- the owner's ID and the hash of
// the stored bytes -- so a client-supplied filename can never influence where
// the file lands.
func imagePath(ownerID int64, sum, ext string) string {
	return strconv.FormatInt(ownerID, 10) + "/" + sum[:2] + "/" + sum + "." + ext
}

// uploadsRoot returns the directory uploaded images are written to.
func (app *App) uploadsRoot() string {
	return filepath.Join(app.cfg.Server.StaticParentDir, staticDir, uploadsDir)
}

// writeUploadedImage stores b at the given uploads-relative path, creating
// any directories it needs.
func (app *App) writeUploadedImage(relPath string, b []byte) error {
	full := filepath.Join(app.uploadsRoot(), filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	return os.WriteFile(full, b, 0644)
}

// removeUploadedImage deletes the file at the given uploads-relative path. A
// file that is already gone is not an error.
func (app *App) removeUploadedImage(relPath string) error {
	full := filepath.Join(app.uploadsRoot(), filepath.FromSlash(relPath))
	err := os.Remove(full)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
