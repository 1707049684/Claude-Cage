package transfermeta

import "io"

// JPEG metadata lives in APPn segments and COM comments. APP0 (JFIF), APP2
// (usually ICC colour management) and APP14 (Adobe colour transform) are kept
// because they affect display, not provenance. EXIF/XMP (APP1), IPTC/Photoshop
// (APP13) and all other APPn/COM segments are dropped.
func sanitizeJPEG(src io.ReadSeeker, dst io.Writer) (bool, error) {
	var soi [2]byte
	if _, err := io.ReadFull(src, soi[:]); err != nil {
		return false, nil
	}
	if soi[0] != 0xFF || soi[1] != 0xD8 {
		return false, nil
	}
	if _, err := dst.Write(soi[:]); err != nil {
		return false, err
	}
	changed := false
	for {
		var prefix [1]byte
		if _, err := io.ReadFull(src, prefix[:]); err != nil {
			return false, nil
		}
		if prefix[0] != 0xFF {
			return false, nil
		}
		var marker [1]byte
		if _, err := io.ReadFull(src, marker[:]); err != nil {
			return false, nil
		}
		switch marker[0] {
		case 0xDA: // start of scan; the remaining entropy-coded data is opaque.
			var length [2]byte
			if _, err := io.ReadFull(src, length[:]); err != nil {
				return false, nil
			}
			if _, err := dst.Write([]byte{0xFF, 0xDA}); err != nil {
				return false, err
			}
			if _, err := dst.Write(length[:]); err != nil {
				return false, err
			}
			if _, err := io.Copy(dst, src); err != nil {
				return false, err
			}
			return changed, nil
		case 0xD9: // end of image; preserve any trailing bytes.
			if _, err := dst.Write([]byte{0xFF, 0xD9}); err != nil {
				return false, err
			}
			if _, err := io.Copy(dst, src); err != nil {
				return false, err
			}
			return changed, nil
		case 0x01, 0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7:
			// TEM and restart markers carry no length field.
			if _, err := dst.Write([]byte{0xFF, marker[0]}); err != nil {
				return false, err
			}
			continue
		}
		var length [2]byte
		if _, err := io.ReadFull(src, length[:]); err != nil {
			return false, nil
		}
		segmentLength := int(length[0])<<8 | int(length[1])
		if segmentLength < 2 {
			return false, nil
		}
		payload := int64(segmentLength - 2)
		if jpegMetadataMarker(marker[0]) {
			if _, err := io.CopyN(io.Discard, src, payload); err != nil {
				return false, err
			}
			changed = true
			continue
		}
		if _, err := dst.Write([]byte{0xFF, marker[0]}); err != nil {
			return false, err
		}
		if _, err := dst.Write(length[:]); err != nil {
			return false, err
		}
		if _, err := io.CopyN(dst, src, payload); err != nil {
			return false, err
		}
	}
}

func jpegMetadataMarker(marker byte) bool {
	if marker == 0xFE { // COM
		return true
	}
	if marker == 0xE0 || marker == 0xE2 || marker == 0xEE { // APP0, APP2, APP14
		return false
	}
	return marker >= 0xE1 && marker <= 0xEF
}
