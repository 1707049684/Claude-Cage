package transfermeta

import (
	"bytes"
	"encoding/binary"
	"io"
)

// PNG text, EXIF and modification-time chunks are provenance metadata. Colour
// chunks (iCCP, sRGB, gAMA, cHRM), physical-size chunks and image data are
// retained so pixels and rendering do not change.
func sanitizePNG(src io.ReadSeeker, dst io.Writer) (bool, error) {
	signature := make([]byte, len(pngSignature))
	if _, err := io.ReadFull(src, signature); err != nil {
		return false, nil
	}
	if !bytes.Equal(signature, pngSignature) {
		return false, nil
	}
	if _, err := dst.Write(signature); err != nil {
		return false, err
	}
	changed := false
	for {
		var header [8]byte
		if _, err := io.ReadFull(src, header[:]); err != nil {
			return false, nil
		}
		length := binary.BigEndian.Uint32(header[:4])
		chunkType := string(header[4:8])
		if int64(length) > MaxBytes {
			return false, nil
		}
		if pngMetadataChunk(chunkType) {
			// Length, payload and CRC are all removed.
			if _, err := io.CopyN(io.Discard, src, int64(length)+4); err != nil {
				return false, err
			}
			changed = true
		} else {
			if _, err := dst.Write(header[:]); err != nil {
				return false, err
			}
			if _, err := io.CopyN(dst, src, int64(length)); err != nil {
				return false, err
			}
			var crc [4]byte
			if _, err := io.ReadFull(src, crc[:]); err != nil {
				return false, nil
			}
			if _, err := dst.Write(crc[:]); err != nil {
				return false, err
			}
		}
		if chunkType == "IEND" {
			if _, err := io.Copy(dst, src); err != nil {
				return false, err
			}
			return changed, nil
		}
	}
}

func pngMetadataChunk(chunkType string) bool {
	switch chunkType {
	case "tEXt", "zTXt", "iTXt", "eXIf", "tIME":
		return true
	}
	return false
}
