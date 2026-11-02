package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
)

// Sizes are the icon sizes baked into every .ico: the tray asks for 16, 20, 24
// or 32 px depending on the monitor scale, 48/64 are used by Explorer's larger
// views, 128/256 by the shell's extra-large previews.
var Sizes = []int{16, 20, 24, 32, 48, 64, 128, 256}

// EncodeICO turns rendered images into a Windows .ico file. Entries up to 64 px
// are stored as classic 32 bpp bitmaps (readable by every Windows version and
// by CreateIconFromResourceEx), larger ones as PNG to keep the file small.
func EncodeICO(images []*image.RGBA) []byte {
	type dirEntry struct {
		w, h   byte
		size   uint32
		offset uint32
	}
	entries := make([]dirEntry, 0, len(images))
	var body bytes.Buffer

	for _, img := range images {
		b := img.Bounds()
		w, h := b.Dx(), b.Dy()
		var data []byte
		if w > 64 {
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				continue
			}
			data = buf.Bytes()
		} else {
			data = encodeDIB(img)
		}
		e := dirEntry{
			w:      byte(w % 256),
			h:      byte(h % 256),
			size:   uint32(len(data)),
			offset: uint32(6 + 16*len(images) + body.Len()),
		}
		entries = append(entries, e)
		body.Write(data)
	}

	le := binary.LittleEndian
	var out bytes.Buffer
	_ = binary.Write(&out, le, uint16(0))            // reserved
	_ = binary.Write(&out, le, uint16(1))            // type: icon
	_ = binary.Write(&out, le, uint16(len(entries))) // image count

	for _, e := range entries {
		out.WriteByte(e.w)
		out.WriteByte(e.h)
		out.WriteByte(0)                       // palette colours
		out.WriteByte(0)                       // reserved
		_ = binary.Write(&out, le, uint16(1))  // planes
		_ = binary.Write(&out, le, uint16(32)) // bits per pixel
		_ = binary.Write(&out, le, e.size)
		_ = binary.Write(&out, le, e.offset)
	}
	out.Write(body.Bytes())
	return out.Bytes()
}

// encodeDIB serialises an image as a 32 bpp bottom-up DIB with the legacy AND
// mask that the ico format expects after the colour data.
func encodeDIB(img *image.RGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var out bytes.Buffer
	le := binary.LittleEndian

	_ = binary.Write(&out, le, uint32(40))    // biSize
	_ = binary.Write(&out, le, int32(w))      // biWidth
	_ = binary.Write(&out, le, int32(h*2))    // biHeight (colour + mask)
	_ = binary.Write(&out, le, uint16(1))     // biPlanes
	_ = binary.Write(&out, le, uint16(32))    // biBitCount
	_ = binary.Write(&out, le, uint32(0))     // biCompression = BI_RGB
	_ = binary.Write(&out, le, uint32(w*h*4)) // biSizeImage
	_ = binary.Write(&out, le, int32(0))      // biXPelsPerMeter
	_ = binary.Write(&out, le, int32(0))      // biYPelsPerMeter
	_ = binary.Write(&out, le, uint32(0))     // biClrUsed
	_ = binary.Write(&out, le, uint32(0))     // biClrImportant

	for y := h - 1; y >= 0; y-- {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w; x++ {
			out.WriteByte(row[x*4+2]) // B
			out.WriteByte(row[x*4+1]) // G
			out.WriteByte(row[x*4+0]) // R
			out.WriteByte(row[x*4+3]) // A
		}
	}

	// AND mask: 1 bit per pixel, rows padded to 4 bytes.
	maskStride := ((w + 31) / 32) * 4
	mask := make([]byte, maskStride*h)
	for y := 0; y < h; y++ {
		row := img.Pix[(h-1-y)*img.Stride : (h-1-y)*img.Stride+w*4]
		for x := 0; x < w; x++ {
			if row[x*4+3] == 0 {
				mask[y*maskStride+x/8] |= 1 << (7 - uint(x%8))
			}
		}
	}
	out.Write(mask)
	return out.Bytes()
}
